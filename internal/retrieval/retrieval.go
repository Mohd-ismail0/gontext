package retrieval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
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
	Action      string // context.search | context.get | context.brief | context.graph
	ResourceID  string // for get/brief/graph seed
	Depth       int    // graph expansion depth (default 1)
	MaxNodes    int    // hard cap on returned nodes
	Predicates  []string
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

	nodes, edges, truncated := p.visibleSubgraph(ctx, principal, req, allowedResourceIDs(citations), 0, nil)

	auditID := platform.NewEventID()
	packet := ports.ContextPacket{
		Version:            PacketVersion,
		PacketID:           platform.NewEventID(),
		OrgID:              req.OrgID,
		Purpose:            req.Purpose,
		RedactionProfile:   pol.RedactionProfile,
		Nodes:              nodes,
		Edges:              edges,
		Citations:          citations,
		Redactions:          redactions,
		Summary:            fmt.Sprintf("%d nodes, %d edges", len(nodes), len(edges)),
		PolicyRevision:     policy.Revision,
		AuthzRevision:      authzRev,
		AuditID:            auditID,
		ActionRestrictions: []string{"outbound.send"},
		Truncated:          truncated,
		NextCursor:         truncationCursor(truncated, edges),
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
	case "context.get", "context.brief", "context.graph":
		need = "context:read"
	case "context.search":
		need = "context:search"
	}
	for _, s := range scopes {
		if s == need {
			return nil
		}
		// Broader read scope satisfies search (OAuth privilege hierarchy).
		if need == "context:search" && s == "context:read" {
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

// Graph expands a seed resource into the caller's visible knowledge subgraph (ADR 0013).
func (p *Pipeline) Graph(ctx context.Context, req Request) (ports.ContextPacket, error) {
	if req.Action == "" {
		req.Action = "context.graph"
	}
	if req.ResourceID == "" {
		return ports.ContextPacket{}, platform.ErrValidation("resource_id required")
	}
	if req.Depth <= 0 {
		req.Depth = 1
	}
	if req.Depth > 4 {
		req.Depth = 4
	}
	if req.MaxNodes <= 0 {
		req.MaxNodes = 50
	}
	if req.MaxNodes > 200 {
		req.MaxNodes = 200
	}
	if req.Limit <= 0 {
		req.Limit = req.MaxNodes
	}
	if req.Consistency == "" {
		req.Consistency = ports.ConsistencyMinLatency
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

	// Delegation constraints (same as Search) — revoke/expiry/purpose before disclosure.
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

	scopes := principal.Scopes
	if len(scopes) == 0 {
		scopes = req.Scopes
	}
	if err := gateScopes(scopes, req.Action, p.AllowEmptyScopes); err != nil {
		return ports.ContextPacket{}, err
	}

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

	pol, err := p.Policy.Evaluate(ctx, ports.PolicyEval{
		Principal:      principal,
		Action:         req.Action,
		Purpose:        req.Purpose,
		Classification: "internal",
		RequestedLimit: req.MaxNodes,
	})
	if err != nil {
		return ports.ContextPacket{}, err
	}
	if !pol.Allow {
		return p.emptyPacket(ctx, principal, req, pol.ReasonCode, nil)
	}

	seedAllow, authzRev, err := p.batchAllow(ctx, principal, req, []string{req.ResourceID})
	if err != nil {
		return ports.ContextPacket{}, err
	}
	if !seedAllow[req.ResourceID] {
		return p.emptyPacket(ctx, principal, req, "AUTHZ_DENY", nil)
	}

	nodes, edges, truncated := p.visibleSubgraph(ctx, principal, req, []string{req.ResourceID}, req.Depth, req.Predicates)
	nodes, edges, trunc2 := truncateGraphPreserveSeeds(nodes, edges, []string{req.ResourceID}, req.MaxNodes)
	if trunc2 {
		truncated = true
	}

	citations := make([]ports.Citation, 0, len(nodes))
	for _, n := range nodes {
		snippet := n.Title
		if snippet == "" {
			snippet = n.ResourceID
		}
		citations = append(citations, ports.Citation{
			CitationID: platform.NewEventID(),
			ResourceID: n.ResourceID,
			RevisionID: n.RevisionID,
			Snippet:    snippet,
		})
	}

	auditID := platform.NewEventID()
	packet := ports.ContextPacket{
		Version:            PacketVersion,
		PacketID:           platform.NewEventID(),
		OrgID:              req.OrgID,
		Purpose:            req.Purpose,
		RedactionProfile:   pol.RedactionProfile,
		Nodes:              nodes,
		Edges:              edges,
		Citations:          citations,
		Redactions:          nil,
		Summary:            fmt.Sprintf("subgraph depth=%d nodes=%d edges=%d", req.Depth, len(nodes), len(edges)),
		PolicyRevision:     policy.Revision,
		AuthzRevision:      authzRev,
		AuditID:            auditID,
		ActionRestrictions: []string{"outbound.send"},
		Truncated:          truncated,
		NextCursor:         truncationCursor(truncated, edges),
		GeneratedAt:        time.Now().UTC(),
	}

	_ = p.Audit.Append(ctx, ports.AuditEvent{
		AuditID:           auditID,
		OrgID:             req.OrgID,
		PrincipalID:       principal.ID,
		PrincipalKind:     principal.Kind,
		Action:            req.Action,
		ReasonCode:        "GRAPH_OK",
		AuthzModelRev:     authzRev,
		PolicyRevision:    policy.Revision,
		ResourceCount:     len(nodes),
		ResourceIDsSample: sampleNodeIDs(nodes, 8),
		Attributes: map[string]string{
			"seed":  req.ResourceID,
			"depth": fmt.Sprintf("%d", req.Depth),
		},
		CreatedAt: time.Now().UTC(),
	})
	return packet, nil
}

func allowedResourceIDs(cites []ports.Citation) []string {
	out := make([]string, 0, len(cites))
	seen := map[string]bool{}
	for _, c := range cites {
		if c.ResourceID == "" || seen[c.ResourceID] {
			continue
		}
		seen[c.ResourceID] = true
		out = append(out, c.ResourceID)
	}
	return out
}

func sampleNodeIDs(nodes []ports.GraphNode, n int) []string {
	out := make([]string, 0, n)
	for _, node := range nodes {
		out = append(out, node.ResourceID)
		if len(out) >= n {
			break
		}
	}
	return out
}

func (p *Pipeline) batchAllow(ctx context.Context, principal ports.Principal, req Request, ids []string) (map[string]bool, string, error) {
	allowed := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return allowed, "", nil
	}
	checks := make([]ports.AuthzCheck, 0, len(ids))
	for _, id := range ids {
		checks = append(checks, ports.AuthzCheck{
			Principal:   principal,
			Action:      "can_read",
			ResourceID:  id,
			Consistency: req.Consistency,
			Delegation:  req.Delegation,
		})
	}
	decisions, err := p.Authz.BatchCheck(ctx, checks)
	if err != nil {
		return nil, "", err
	}
	authzRev := ""
	for i, id := range ids {
		if i < len(decisions) && decisions[i].Allowed {
			allowed[id] = true
			authzRev = decisions[i].ModelRevision
		}
	}
	return allowed, authzRev, nil
}

// visibleSubgraph returns the caller's final visible subgraph after AuthZ and policy.
// Edges are filtered against the surviving node set only (ADR 0013).
// Nodes are returned in BFS discovery order (seeds first) for stable truncation.
func (p *Pipeline) visibleSubgraph(ctx context.Context, principal ports.Principal, req Request, seeds []string, depth int, predicates []string) ([]ports.GraphNode, []ports.GraphEdge, bool) {
	const edgeCap = 500
	truncated := false

	// BatchCheck seeds first — never trust caller-supplied IDs as already authorized.
	seedIDs := make([]string, 0, len(seeds))
	seenSeed := map[string]bool{}
	for _, id := range seeds {
		if id == "" || seenSeed[id] {
			continue
		}
		seenSeed[id] = true
		seedIDs = append(seedIDs, id)
	}
	sort.Strings(seedIDs)
	authzOK := map[string]bool{}
	discovery := make([]string, 0, len(seedIDs))
	if len(seedIDs) > 0 {
		ok, _, err := p.batchAllow(ctx, principal, req, seedIDs)
		if err != nil {
			return nil, nil, false
		}
		for _, id := range seedIDs {
			if ok[id] {
				authzOK[id] = true
				discovery = append(discovery, id)
			}
		}
	}
	frontier := append([]string{}, discovery...)

	for hop := 0; hop < depth; hop++ {
		if len(frontier) == 0 {
			break
		}
		ledgerEdges, err := p.Ledger.ListEdges(ctx, req.OrgID, ports.EdgeListOptions{
			ResourceIDs: frontier,
			Predicates:  predicates,
			Limit:       edgeCap + 1,
		})
		if err != nil {
			break
		}
		if len(ledgerEdges) > edgeCap {
			truncated = true
			ledgerEdges = ledgerEdges[:edgeCap]
		}
		neighborSet := map[string]bool{}
		var neighbors []string
		for _, e := range ledgerEdges {
			if e.State == "TOMBSTONED" {
				continue
			}
			for _, end := range []string{e.FromID, e.ToID} {
				if !authzOK[end] && !neighborSet[end] {
					neighborSet[end] = true
					neighbors = append(neighbors, end)
				}
			}
		}
		sort.Strings(neighbors)
		if len(neighbors) == 0 {
			break
		}
		ok, _, err := p.batchAllow(ctx, principal, req, neighbors)
		if err != nil {
			break
		}
		next := make([]string, 0, len(neighbors))
		for _, id := range neighbors {
			if ok[id] {
				authzOK[id] = true
				discovery = append(discovery, id)
				next = append(next, id)
			}
		}
		frontier = next
	}

	// Hydrate + policy in discovery order.
	visible := map[string]ports.GraphNode{}
	orderedIDs := make([]string, 0, len(discovery))
	for _, id := range discovery {
		rec, err := p.Ledger.GetRecord(ctx, req.OrgID, id)
		if err != nil || deletion.IsTombstoned(rec.State) {
			continue
		}
		if rec.State == ports.LifecyclePlaceholder || rec.State == ports.LifecycleEnsured {
			continue // non-retrievable placeholders
		}
		rpol, err := p.Policy.Evaluate(ctx, ports.PolicyEval{
			Principal:      principal,
			Action:         req.Action,
			Purpose:        req.Purpose,
			Classification: rec.Classification,
			Record:         &rec,
		})
		if err != nil || !rpol.Allow {
			continue
		}
		visible[id] = ports.GraphNode{
			ResourceID:     rec.ResourceID,
			Kind:           rec.Kind,
			Title:          rec.Title,
			Classification: rec.Classification,
			Labels:         rec.Labels,
			RevisionID:     rec.CurrentRevID,
			State:          rec.State,
			Attributes:     sanitizeNodeAttrs(rec.Attributes),
		}
		orderedIDs = append(orderedIDs, id)
	}

	ledgerEdges, err := p.Ledger.ListEdges(ctx, req.OrgID, ports.EdgeListOptions{
		ResourceIDs: orderedIDs,
		Predicates:  predicates,
		Limit:       edgeCap + 1,
	})
	var finalEdges []ports.GraphEdge
	if err == nil {
		if len(ledgerEdges) > edgeCap {
			truncated = true
			ledgerEdges = ledgerEdges[:edgeCap]
		}
		seen := map[string]bool{}
		for _, e := range ledgerEdges {
			if e.State == "TOMBSTONED" || seen[e.EdgeID] {
				continue
			}
			if _, okFrom := visible[e.FromID]; !okFrom {
				continue
			}
			if _, okTo := visible[e.ToID]; !okTo {
				continue
			}
			seen[e.EdgeID] = true
			finalEdges = append(finalEdges, e)
		}
	}
	sort.Slice(finalEdges, func(i, j int) bool {
		if finalEdges[i].FromID != finalEdges[j].FromID {
			return finalEdges[i].FromID < finalEdges[j].FromID
		}
		if finalEdges[i].Predicate != finalEdges[j].Predicate {
			return finalEdges[i].Predicate < finalEdges[j].Predicate
		}
		return finalEdges[i].ToID < finalEdges[j].ToID
	})

	nodes := make([]ports.GraphNode, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		nodes = append(nodes, visible[id])
	}
	return nodes, finalEdges, truncated
}

// truncateGraphPreserveSeeds caps node count while always retaining authorized seeds,
// then fills remaining slots in BFS discovery order (industry: seed-stable neighborhood pages).
func truncateGraphPreserveSeeds(nodes []ports.GraphNode, edges []ports.GraphEdge, seeds []string, maxNodes int) ([]ports.GraphNode, []ports.GraphEdge, bool) {
	if maxNodes <= 0 || len(nodes) <= maxNodes {
		return nodes, edges, false
	}
	seedSet := map[string]bool{}
	for _, s := range seeds {
		if s != "" {
			seedSet[s] = true
		}
	}
	keep := map[string]bool{}
	out := make([]ports.GraphNode, 0, maxNodes)
	// Pass 1: seeds present in the result.
	for _, n := range nodes {
		if seedSet[n.ResourceID] && !keep[n.ResourceID] {
			keep[n.ResourceID] = true
			out = append(out, n)
			if len(out) >= maxNodes {
				break
			}
		}
	}
	// Pass 2: BFS/discovery order for the remainder.
	if len(out) < maxNodes {
		for _, n := range nodes {
			if keep[n.ResourceID] {
				continue
			}
			keep[n.ResourceID] = true
			out = append(out, n)
			if len(out) >= maxNodes {
				break
			}
		}
	}
	filtered := edges[:0]
	for _, e := range edges {
		if keep[e.FromID] && keep[e.ToID] {
			filtered = append(filtered, e)
		}
	}
	return out, filtered, true
}

func sanitizeNodeAttrs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range in {
		switch k {
		case "visibility_ref", "placeholder", "ensured", "signing_secret", "mapping_spec":
			continue
		default:
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func truncationCursor(truncated bool, edges []ports.GraphEdge) string {
	if !truncated || len(edges) == 0 {
		return ""
	}
	return "edge:" + edges[len(edges)-1].EdgeID
}
