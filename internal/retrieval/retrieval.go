package retrieval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/xsama/context-fabric/internal/audit"
	"github.com/xsama/context-fabric/internal/deletion"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/policy"
	"github.com/xsama/context-fabric/internal/ports"
)

const PacketVersion = "context-fabric.packet/v1"

// Pipeline owns the policy-first retrieval path.
// Never global top-K then filter: index candidates are scoped by mandatory predicates,
// then exact BatchCheck before hydration.
type Pipeline struct {
	Identity ports.IdentityProvider
	Authz    ports.AuthorizationProvider
	Policy   ports.PolicyProvider
	Ledger   ports.LedgerStore
	Index    ports.IndexProvider
	Audit    audit.Logger
	// Optional snippet source (memory index); may be nil.
	Snippets SnippetSource
	// AllowEmptyScopes is a test helper; production must leave this false.
	AllowEmptyScopes bool
}

// SnippetSource returns retrieval-safe text for citations.
type SnippetSource interface {
	GetDocument(orgID, resourceID string) (ports.IndexDocument, bool)
}

// Request is a retrieval invocation after HTTP/MCP decoding.
type Request struct {
	Credentials ports.Credentials
	OrgID       string
	Query       string
	Purpose     string
	Limit       int
	Filters     map[string]string // include_tags (comma), context_space_id, resource_id, ...
	Consistency ports.ConsistencyMode
	Scopes      []string
	Action      string // context.search | context.get | context.brief
	ResourceID  string // for get/brief
	Delegation  *ports.DelegationGrant
}

// Search runs the full governed retrieval pipeline and returns a ContextPacket.
func (p *Pipeline) Search(ctx context.Context, req Request) (ports.ContextPacket, error) {
	if req.Action == "" {
		req.Action = "context.search"
	}
	principal, err := p.Identity.Authenticate(ctx, req.Credentials)
	if err != nil {
		return ports.ContextPacket{}, err
	}
	if principal.OrgID == "" {
		return ports.ContextPacket{}, platform.ErrForbidden("principal missing organization")
	}
	if req.OrgID == "" {
		req.OrgID = principal.OrgID
	} else if principal.OrgID != req.OrgID {
		return ports.ContextPacket{}, platform.ErrForbidden("organization mismatch")
	}

	if req.Consistency == "" {
		req.Consistency = ports.ConsistencyMinLatency
	}
	if req.Limit <= 0 {
		req.Limit = 12
	}
	if req.Limit > 50 {
		req.Limit = 50
	}

	// Scope / action gate (OAuth scope ceiling): prefer IdentityProvider scopes.
	scopes := principal.Scopes
	if len(scopes) == 0 {
		scopes = req.Scopes
	}
	if err := gateScopes(scopes, req.Action, p.AllowEmptyScopes); err != nil {
		return ports.ContextPacket{}, err
	}

	// Resolve delegation constraints (intersection).
	if req.Delegation != nil {
		if req.Delegation.Revoked {
			return ports.ContextPacket{}, platform.ErrForbidden("delegation revoked")
		}
		if req.Delegation.ExpiresAt != nil && time.Now().After(*req.Delegation.ExpiresAt) {
			return ports.ContextPacket{}, platform.ErrForbidden("delegation expired")
		}
		if len(req.Delegation.Purposes) > 0 && !contains(req.Delegation.Purposes, req.Purpose) {
			return ports.ContextPacket{}, platform.ErrForbidden("purpose outside delegation")
		}
	}

	// Coarse AuthZ scope.
	scope, err := p.Authz.ResolveCandidateScope(ctx, ports.ScopeResolve{
		Principal:   principal,
		Action:      req.Action,
		Consistency: req.Consistency,
	})
	if err != nil {
		return ports.ContextPacket{}, err
	}
	if scope.ReasonCode == "AUTHZ_SCOPE_DENY" {
		return p.emptyPacket(ctx, principal, req, "AUTHZ_SCOPE_DENY", nil)
	}

	// Policy obligations (purpose/classification ceiling) — never grants access.
	pol, err := p.Policy.Evaluate(ctx, ports.PolicyEval{
		Principal:      principal,
		Action:         req.Action,
		Purpose:        req.Purpose,
		Classification: "internal",
		RequestedLimit: req.Limit,
	})
	if err != nil {
		return ports.ContextPacket{}, err
	}
	if !pol.Allow {
		return p.emptyPacket(ctx, principal, req, pol.ReasonCode, nil)
	}
	if pol.MaxResults > 0 && pol.MaxResults < req.Limit {
		req.Limit = pol.MaxResults
	}

	// Mandatory server-generated predicate (org/purpose/classification/tags AND-narrow).
	// Tags never grant access — they only further constrain candidates.
	filters := map[string]string{
		"purpose":                req.Purpose,
		"classification_ceiling": "restricted", // AuthZ still decides; index may prefilter
	}
	for k, v := range req.Filters {
		if k == "include_tags" || k == "context_space_id" || k == "resource_id" || k == "brand_id" {
			filters[k] = v
		}
	}
	if req.ResourceID != "" {
		filters["resource_id"] = req.ResourceID
	}

	query := req.Query
	if query == "" && req.ResourceID != "" {
		query = req.ResourceID
	}

	candidates, err := p.Index.SearchCandidates(ctx, req.OrgID, query, req.Limit, filters)
	if err != nil {
		return ports.ContextPacket{}, err
	}

	// Exact BatchCheck before hydration — never hydrate then filter.
	checks := make([]ports.AuthzCheck, 0, len(candidates))
	for _, hit := range candidates {
		checks = append(checks, ports.AuthzCheck{
			Principal:   principal,
			Action:      "can_read",
			ResourceID:  hit.ResourceID,
			Consistency: req.Consistency,
			Delegation:  req.Delegation,
		})
	}
	decisions, err := p.Authz.BatchCheck(ctx, checks)
	if err != nil {
		return ports.ContextPacket{}, err
	}

	allowedHits := make([]ports.SearchHit, 0, len(candidates))
	authzRev := ""
	for i, hit := range candidates {
		if i < len(decisions) && decisions[i].Allowed {
			allowedHits = append(allowedHits, hit)
			authzRev = decisions[i].ModelRevision
		}
	}

	citations := make([]ports.Citation, 0, len(allowedHits))
	redactions := make([]ports.Redaction, 0)
	for _, hit := range allowedHits {
		rec, err := p.Ledger.GetRecord(ctx, req.OrgID, hit.ResourceID)
		if err != nil {
			continue
		}
		// Tombstones dominate any older upsert, including after index lag/restore.
		if deletion.IsTombstoned(rec.State) {
			continue
		}
		// Per-record policy (classification obligations).
		rpol, err := p.Policy.Evaluate(ctx, ports.PolicyEval{
			Principal:      principal,
			Action:         req.Action,
			Purpose:        req.Purpose,
			Classification: rec.Classification,
			Record:         &rec,
			RequestedLimit: req.Limit,
		})
		if err != nil || !rpol.Allow {
			continue
		}
		snippet := redactSnippet(loadSnippet(p, req.OrgID, hit.ResourceID, rec), rpol.RedactionProfile)
		revID := hit.RevisionID
		if revID == "" {
			revID = rec.CurrentRevID
		}
		citations = append(citations, ports.Citation{
			CitationID: platform.NewEventID(),
			ResourceID: hit.ResourceID,
			RevisionID: revID,
			Snippet:    snippet,
			Score:      hit.Score,
		})
		if rpol.RedactionProfile != "" && rpol.RedactionProfile != "none" {
			redactions = append(redactions, ports.Redaction{
				Profile:    rpol.RedactionProfile,
				ReasonCode: rpol.ReasonCode,
			})
		}
	}

	auditID := platform.NewEventID()
	packet := ports.ContextPacket{
		Version:            PacketVersion,
		PacketID:           platform.NewEventID(),
		OrgID:              req.OrgID,
		Purpose:            req.Purpose,
		RedactionProfile:   pol.RedactionProfile,
		Citations:          citations,
		Redactions:          redactions,
		Summary:            fmt.Sprintf("%d cited resources", len(citations)),
		PolicyRevision:     policy.Revision,
		AuthzRevision:      authzRev,
		AuditID:            auditID,
		ActionRestrictions: []string{"outbound.send"},
		GeneratedAt:        time.Now().UTC(),
	}

	_ = p.Audit.Append(ctx, ports.AuditEvent{
		AuditID:           auditID,
		OrgID:             req.OrgID,
		PrincipalID:       principal.ID,
		PrincipalKind:     principal.Kind,
		DelegationID:      delegationID(req.Delegation),
		Action:            req.Action,
		ReasonCode:        "RETRIEVAL_OK",
		AuthzModelRev:     authzRev,
		PolicyRevision:    policy.Revision,
		ResourceCount:     len(citations),
		ResourceIDsSample: sampleIDs(citations, 8),
		Attributes: map[string]string{
			"query_hash": hashQuery(req.Query),
			"purpose":    req.Purpose,
		},
		CreatedAt: time.Now().UTC(),
	})

	return packet, nil
}

func (p *Pipeline) emptyPacket(ctx context.Context, principal ports.Principal, req Request, reason string, _ error) (ports.ContextPacket, error) {
	auditID := platform.NewEventID()
	_ = p.Audit.Append(ctx, ports.AuditEvent{
		AuditID:       auditID,
		OrgID:         req.OrgID,
		PrincipalID:   principal.ID,
		PrincipalKind: principal.Kind,
		Action:        req.Action,
		ReasonCode:    reason,
		CreatedAt:     time.Now().UTC(),
	})
	return ports.ContextPacket{
		Version:            PacketVersion,
		OrgID:              req.OrgID,
		Purpose:            req.Purpose,
		Citations:          []ports.Citation{},
		Redactions:          []ports.Redaction{},
		PolicyRevision:     policy.Revision,
		AuthzRevision:      "",
		AuditID:            auditID,
		ActionRestrictions: []string{"context.search", "context.get"},
		GeneratedAt:        time.Now().UTC(),
	}, nil
}

func gateScopes(scopes []string, action string, allowEmpty bool) error {
	if len(scopes) == 0 {
		if allowEmpty {
			return nil
		}
		return platform.ErrForbidden("missing scopes")
	}
	need := "context:search"
	switch action {
	case "context.get", "context.brief":
		need = "context:read"
	case "context.search":
		need = "context:search"
	}
	for _, s := range scopes {
		if s == need || s == "context:read" || s == "context:search" {
			return nil
		}
	}
	return platform.ErrForbidden("missing oauth scope " + need)
}

func loadSnippet(p *Pipeline, orgID, resourceID string, rec ports.Record) string {
	if p.Snippets != nil {
		if d, ok := p.Snippets.GetDocument(orgID, resourceID); ok && d.Text != "" {
			return truncate(d.Text, 240)
		}
	}
	if rec.Title != "" {
		return rec.Title
	}
	return resourceID
}

func redactSnippet(s, profile string) string {
	if profile == "pii_mask" || profile == "mask_pii" {
		return strings.ReplaceAll(s, "@", "[at]")
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func delegationID(d *ports.DelegationGrant) string {
	if d == nil {
		return ""
	}
	return d.ID
}

func sampleIDs(cites []ports.Citation, n int) []string {
	out := make([]string, 0, n)
	for _, c := range cites {
		out = append(out, c.ResourceID)
		if len(out) >= n {
			break
		}
	}
	return out
}

func hashQuery(q string) string {
	sum := sha256.Sum256([]byte(q))
	return hex.EncodeToString(sum[:8])
}
