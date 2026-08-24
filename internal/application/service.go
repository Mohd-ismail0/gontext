package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/xsama/context-fabric/internal/audit"
	"github.com/xsama/context-fabric/internal/changes"
	"github.com/xsama/context-fabric/internal/deletion"
	"github.com/xsama/context-fabric/internal/export"
	"github.com/xsama/context-fabric/internal/ingest"
	"github.com/xsama/context-fabric/internal/mapping"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
	"github.com/xsama/context-fabric/internal/quota"
	"github.com/xsama/context-fabric/internal/retrieval"
)

// ApplicationService wires ports into the public REST/MCP operations.
type ApplicationService struct {
	Identity ports.IdentityProvider
	Authz    ports.AuthorizationProvider
	Policy   ports.PolicyProvider
	Ledger   ports.LedgerStore
	Evidence ports.EvidenceStore
	Index    ports.IndexProvider
	Bus      ports.EventBus
	Audit    audit.Logger
	Quota    *quota.Limiter
	Retrieve *retrieval.Pipeline
	Ingest   *ingest.IntakeService
	Build    VersionInfo
	Ready    func() bool
	// ReadyDetail, when set, supplies OpenAPI ReadyStatus checks (migrations, authz, …).
	ReadyDetail func() (ready bool, checks map[string]any)

	// Optional extended store methods (memory/postgres).
	Changes ChangeLister
	Quotas  QuotaStore
	Extras  ExtraStore

	// Lifecycle / MCP extensions.
	Deletion    *deletion.Service
	Export      *export.Service
	ChangeFeed  *changes.Service
	Credentials ports.CredentialProvider
	ExportJobs  ExportJobStore

	// MappingBySource optionally resolves MappingSpec by source id.
	MappingBySource func(ctx context.Context, orgID, sourceID string) (mapping.Spec, error)

	// Mappings persists/loads MappingSpec by source.
	Mappings MappingStore
}

// MappingStore persists MappingSpec documents keyed by source.
type MappingStore interface {
	PutMappingSpec(ctx context.Context, orgID, sourceID string, spec mapping.Spec) error
	GetMappingSpec(ctx context.Context, orgID, sourceID string) (mapping.Spec, error)
}

// VersionInfo is returned by Version().
type VersionInfo struct {
	ProductVersion      string `json:"product_version"`
	APIVersion          string `json:"api_version"`
	PacketVersion       string `json:"packet_version"`
	ExportFormatVersion string `json:"export_format_version"`
	GitSHA              string `json:"git_sha,omitempty"`
	AuthzModelID        string `json:"authz_model_id,omitempty"`
}

// ChangeLister lists org-scoped change events.
type ChangeLister interface {
	ListChanges(ctx context.Context, orgID, cursor string, limit int) ([]ports.ChangeEvent, string, error)
	AppendChange(ctx context.Context, ev ports.ChangeEvent) error
}

// QuotaStore gets/sets org quotas.
type QuotaStore interface {
	GetQuotas(ctx context.Context, orgID string) (ports.Quota, error)
	SetQuotas(ctx context.Context, orgID string, q ports.Quota) error
}

// ExtraStore holds webhooks/access/export helpers used by memory adapter.
type ExtraStore interface {
	ListAudit(ctx context.Context, orgID string, limit int) ([]ports.AuditEvent, error)
	PutAccessRequest(ctx context.Context, orgID, requestID, resourceID, purpose, justification, auditID string) error
}

// ExportJobStore persists export job manifests.
type ExportJobStore interface {
	PutExportJob(ctx context.Context, orgID, jobID, status string, manifest any) error
	GetExportJob(ctx context.Context, orgID, jobID string) (jobIDOut string, status string, manifest any, err error)
}

// SearchRequest is the HTTP/MCP search body.
type SearchRequest struct {
	Query       string         `json:"query"`
	Purpose     string         `json:"purpose"`
	Scope       map[string]any `json:"scope,omitempty"`
	Filters     *SearchFilters `json:"filters,omitempty"`
	MaxItems    int            `json:"max_items"`
	Consistency string         `json:"consistency"`
}

// SearchFilters are AND-narrow only; never grant access.
type SearchFilters struct {
	IncludeTags []string `json:"include_tags,omitempty"`
}

func (s *ApplicationService) Search(ctx context.Context, creds ports.Credentials, orgID string, scopes []string, body SearchRequest) (ports.ContextPacket, error) {
	if s.Quota != nil {
		if err := s.Quota.Allow(quota.Key{OrgID: orgID, Op: quota.OpSearch}); err != nil {
			return ports.ContextPacket{}, err
		}
	}
	filters := map[string]string{}
	if body.Filters != nil && len(body.Filters.IncludeTags) > 0 {
		filters["include_tags"] = joinComma(body.Filters.IncludeTags)
	}
	if body.Scope != nil {
		if cs, ok := body.Scope["context_space_id"].(string); ok {
			filters["context_space_id"] = cs
		}
	}
	cons := ports.ConsistencyMinLatency
	if body.Consistency == string(ports.ConsistencyFullyConsistent) {
		cons = ports.ConsistencyFullyConsistent
	}
	return s.Retrieve.Search(ctx, retrieval.Request{
		Credentials: creds,
		OrgID:       orgID,
		Query:       body.Query,
		Purpose:     body.Purpose,
		Limit:       body.MaxItems,
		Filters:     filters,
		Consistency: cons,
		Scopes:      scopes,
		Action:      "context.search",
	})
}

func (s *ApplicationService) GetResource(ctx context.Context, creds ports.Credentials, orgID, resourceID, purpose, consistency string, scopes []string) (ports.ContextPacket, error) {
	cons := ports.ConsistencyMinLatency
	if consistency == string(ports.ConsistencyFullyConsistent) {
		cons = ports.ConsistencyFullyConsistent
	}
	return s.Retrieve.Search(ctx, retrieval.Request{
		Credentials: creds,
		OrgID:       orgID,
		Query:       resourceID,
		Purpose:     purpose,
		Limit:       1,
		Filters:     map[string]string{"resource_id": resourceID},
		Consistency: cons,
		Scopes:      scopes,
		Action:      "context.get",
		ResourceID:  resourceID,
	})
}

func (s *ApplicationService) Brief(ctx context.Context, creds ports.Credentials, orgID string, scopes []string, purpose, resourceID, consistency string, maxItems int) (ports.ContextPacket, error) {
	cons := ports.ConsistencyMinLatency
	if consistency == string(ports.ConsistencyFullyConsistent) {
		cons = ports.ConsistencyFullyConsistent
	}
	return s.Retrieve.Search(ctx, retrieval.Request{
		Credentials: creds,
		OrgID:       orgID,
		Query:       resourceID,
		Purpose:     purpose,
		Limit:       maxItems,
		Filters:     map[string]string{"resource_id": resourceID},
		Consistency: cons,
		Scopes:      scopes,
		Action:      "context.brief",
		ResourceID:  resourceID,
	})
}

// GraphRequest is the HTTP/MCP neighborhood body.
type GraphRequest struct {
	ResourceID  string   `json:"resource_id"`
	Purpose     string   `json:"purpose"`
	Depth       int      `json:"depth"`
	MaxNodes    int      `json:"max_nodes"`
	Predicates  []string `json:"predicates,omitempty"`
	Consistency string   `json:"consistency"`
}

// Graph returns the caller's visible knowledge subgraph around a seed node.
func (s *ApplicationService) Graph(ctx context.Context, creds ports.Credentials, orgID string, scopes []string, body GraphRequest) (ports.ContextPacket, error) {
	if s.Quota != nil {
		if err := s.Quota.Allow(quota.Key{OrgID: orgID, Op: quota.OpSearch}); err != nil {
			return ports.ContextPacket{}, err
		}
	}
	cons := ports.ConsistencyMinLatency
	if body.Consistency == string(ports.ConsistencyFullyConsistent) {
		cons = ports.ConsistencyFullyConsistent
	}
	return s.Retrieve.Graph(ctx, retrieval.Request{
		Credentials: creds,
		OrgID:       orgID,
		Purpose:     body.Purpose,
		Consistency: cons,
		Scopes:      scopes,
		Action:      "context.graph",
		ResourceID:  body.ResourceID,
		Depth:       body.Depth,
		MaxNodes:    body.MaxNodes,
		Predicates:  body.Predicates,
	})
}

func (s *ApplicationService) RequestAccess(ctx context.Context, creds ports.Credentials, orgID string, resourceID, purpose, justification string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	id := platform.NewEventID()
	auditID := platform.NewEventID()
	_ = s.Audit.Append(ctx, ports.AuditEvent{
		AuditID: auditID, OrgID: orgID, PrincipalID: principal.ID, PrincipalKind: principal.Kind,
		Action: "context.request_access", ReasonCode: "ACCESS_REQUEST_PENDING", CreatedAt: time.Now().UTC(),
		Attributes: map[string]string{"resource_id": resourceID, "purpose": purpose},
	})
	if s.Extras != nil {
		_ = s.Extras.PutAccessRequest(ctx, orgID, id, resourceID, purpose, justification, auditID)
	}
	return map[string]any{
		"request_id":    id,
		"status":        "pending",
		"resource_id":   resourceID,
		"purpose":       purpose,
		"justification": justification,
		"created_at":    time.Now().UTC(),
		"audit_id":      auditID,
	}, nil
}

func (s *ApplicationService) RegisterSource(ctx context.Context, creds ports.Credentials, orgID string, src ports.SourceRegistration) (ports.SourceRegistration, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return ports.SourceRegistration{}, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return ports.SourceRegistration{}, err
	}
	if src.SourceID == "" {
		src.SourceID = platform.NewEventID()
	}
	src.OrgID = orgID
	if src.TrustCeiling == "" {
		src.TrustCeiling = src.TrustTier
	}
	if src.TrustTier == "" {
		src.TrustTier = src.TrustCeiling
	}
	if src.AuthorityCeiling == "" {
		src.AuthorityCeiling = "source_of_truth"
	}
	if src.ClassificationCeiling == "" {
		src.ClassificationCeiling = src.EffectiveClassificationCeiling()
	}
	if src.ReplayWindowSeconds <= 0 {
		src.ReplayWindowSeconds = ingest.DefaultReplay
	}
	if src.SigningSecret != "" {
		if src.Attributes == nil {
			src.Attributes = map[string]string{}
		}
		src.Attributes["signing_secret"] = src.SigningSecret
	}

	var inline *mapping.Spec
	if len(src.MappingSpecInline) > 0 {
		spec, err := mapping.ParseSpec(src.MappingSpecInline)
		if err != nil {
			return ports.SourceRegistration{}, err
		}
		if spec.ID == "" {
			spec.ID = src.MappingSpec
		}
		if spec.ID == "" {
			spec.ID = src.SourceID
		}
		if src.MappingSpec == "" {
			src.MappingSpec = spec.ID
		}
		spec.OrganizationID = orgID
		spec.SourceID = src.SourceID
		inline = &spec
	}

	inlineBytes := src.MappingSpecInline
	src.MappingSpecInline = nil
	err = s.Ledger.WithOrgTx(ctx, orgID, func(ctx context.Context, tx ports.Tx) error {
		return s.Ledger.UpsertSource(ctx, tx, src)
	})
	if err != nil {
		return ports.SourceRegistration{}, err
	}
	if inline != nil {
		if s.Mappings != nil {
			_ = s.Mappings.PutMappingSpec(ctx, orgID, src.SourceID, *inline)
		} else if putter, ok := s.Ledger.(MappingStore); ok {
			_ = putter.PutMappingSpec(ctx, orgID, src.SourceID, *inline)
		}
	}
	// Return signing_secret once on create; strip inline mapping blob.
	src.MappingSpecInline = nil
	_ = inlineBytes
	return src, nil
}

func (s *ApplicationService) VerifySource(ctx context.Context, creds ports.Credentials, orgID, sourceID string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	src, err := s.Ledger.GetSource(ctx, orgID, sourceID)
	if err != nil {
		return nil, err
	}
	secret := src.SigningSecret
	if secret == "" && src.Attributes != nil {
		secret = src.Attributes["signing_secret"]
	}
	checkedAt := time.Now().UTC()
	if secret == "" {
		return map[string]any{
			"source_id":  src.SourceID,
			"system":     src.System,
			"enabled":    src.Enabled,
			"status":     "failed",
			"reason_code": "missing_signing_secret",
			"detail":     "signing_secret is empty",
			"checked_at": checkedAt,
		}, nil
	}
	probe := []byte("context-fabric-verify-probe")
	sig := ingest.SignHMAC(secret, probe)
	if err := ingest.VerifyHMAC(secret, probe, sig); err != nil {
		return map[string]any{
			"source_id":   src.SourceID,
			"system":      src.System,
			"enabled":     src.Enabled,
			"status":      "failed",
			"reason_code": "hmac_self_test_failed",
			"detail":      err.Error(),
			"checked_at":  checkedAt,
		}, nil
	}
	return map[string]any{
		"source_id":  src.SourceID,
		"system":     src.System,
		"enabled":    src.Enabled,
		"status":     "ok",
		"checked_at": checkedAt,
	}, nil
}

func (s *ApplicationService) ListSources(ctx context.Context, creds ports.Credentials, orgID string) ([]ports.SourceRegistration, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	items, err := s.Ledger.ListSources(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i] = redactSource(items[i])
	}
	return items, nil
}

func (s *ApplicationService) GetSource(ctx context.Context, creds ports.Credentials, orgID, sourceID string) (ports.SourceRegistration, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return ports.SourceRegistration{}, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return ports.SourceRegistration{}, err
	}
	src, err := s.Ledger.GetSource(ctx, orgID, sourceID)
	if err != nil {
		return ports.SourceRegistration{}, err
	}
	return redactSource(src), nil
}

func (s *ApplicationService) RotateSourceSecret(ctx context.Context, creds ports.Credentials, orgID, sourceID string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	src, err := s.Ledger.GetSource(ctx, orgID, sourceID)
	if err != nil {
		return nil, err
	}
	secret, err := randomSigningSecret()
	if err != nil {
		return nil, err
	}
	keyID := platform.NewEventID()
	if src.Attributes == nil {
		src.Attributes = map[string]string{}
	}
	src.Attributes["signing_secret"] = secret
	src.Attributes["signing_key_id"] = keyID
	src.SigningSecret = secret
	src.UpdatedAt = time.Now().UTC()
	err = s.Ledger.WithOrgTx(ctx, orgID, func(ctx context.Context, tx ports.Tx) error {
		return s.Ledger.UpsertSource(ctx, tx, src)
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"source_id":   src.SourceID,
		"key_id":      keyID,
		"secret_once": secret,
	}, nil
}

func redactSource(src ports.SourceRegistration) ports.SourceRegistration {
	src.SigningSecret = ""
	src.MappingSpecInline = nil
	if src.Attributes != nil {
		attrs := make(map[string]string, len(src.Attributes))
		for k, v := range src.Attributes {
			if k == "signing_secret" {
				continue
			}
			attrs[k] = v
		}
		src.Attributes = attrs
	}
	return src
}

func randomSigningSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "cfsrc_" + hex.EncodeToString(b), nil
}

// IntakeRequest is the HTTP/gateway intake envelope.
type IntakeRequest struct {
	Event          ports.IntakeEvent
	Body           []byte
	Signature      string
	Timestamp      string
	IdempotencyKey string
	SourceID       string
	Payload        map[string]any
	DryRun         bool
	SkipHMAC       bool
	Mapping        mapping.Spec
}

func (s *ApplicationService) Intake(ctx context.Context, creds ports.Credentials, orgID string, req IntakeRequest) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	if s.Quota != nil {
		if err := s.Quota.Allow(quota.Key{OrgID: orgID, Op: quota.OpIntake}); err != nil {
			return nil, err
		}
	}
	event := req.Event
	if event.EventID == "" {
		event.EventID = platform.NewEventID()
	}
	event.OrgID = orgID
	if event.IdempotencyKey == "" {
		event.IdempotencyKey = req.IdempotencyKey
	}
	sourceID := req.SourceID
	if sourceID == "" {
		sourceID = event.SourceSystem
	}

	ing := s.Ingest
	if ing == nil {
		ing = &ingest.IntakeService{Ledger: s.Ledger, Evidence: s.Evidence, Authz: asRelationshipWriter(s.Authz)}
	} else if ing.Authz == nil {
		ing.Authz = asRelationshipWriter(s.Authz)
	}

	spec := req.Mapping
	if spec.ID == "" && s.MappingBySource != nil && sourceID != "" {
		if m, err := s.MappingBySource(ctx, orgID, sourceID); err == nil {
			spec = m
		}
	}

	result, err := ing.Intake(ctx, ingest.Request{
		OrgID:          orgID,
		SourceID:       sourceID,
		Body:           req.Body,
		Signature:      req.Signature,
		Timestamp:      req.Timestamp,
		IdempotencyKey: event.IdempotencyKey,
		Mapping:        spec,
		Payload:        req.Payload,
		Event:          event,
		DryRun:         req.DryRun,
		SkipHMAC:       req.SkipHMAC,
	})
	if err != nil {
		return nil, err
	}

	if !result.Duplicate && !req.DryRun {
		if s.ChangeFeed != nil {
			if err := s.ChangeFeed.Append(ctx, ports.ChangeEvent{
				EventID: platform.NewEventID(), OrgID: orgID, ResourceID: result.ResourceID,
				RevisionID: result.RevisionID, Action: "resource.accepted", Cursor: result.RevisionID, OccurredAt: time.Now().UTC(),
			}); err != nil {
				return nil, platform.ErrUnavailable("change feed append failed: " + err.Error())
			}
			if result.EdgeCount > 0 {
				if err := s.ChangeFeed.Append(ctx, ports.ChangeEvent{
					EventID: platform.NewEventID(), OrgID: orgID, ResourceID: result.ResourceID,
					RevisionID: result.RevisionID, Action: "graph.edges_upserted",
					Cursor: result.RevisionID + ":edges", OccurredAt: time.Now().UTC(),
				}); err != nil {
					return nil, platform.ErrUnavailable("change feed edge append failed: " + err.Error())
				}
			}
		} else if s.Changes != nil {
			if err := s.Changes.AppendChange(ctx, ports.ChangeEvent{
				EventID: platform.NewEventID(), OrgID: orgID, ResourceID: result.ResourceID,
				RevisionID: result.RevisionID, Action: "upsert", Cursor: result.RevisionID, OccurredAt: time.Now().UTC(),
			}); err != nil {
				return nil, platform.ErrUnavailable("change append failed: " + err.Error())
			}
			if result.EdgeCount > 0 {
				if err := s.Changes.AppendChange(ctx, ports.ChangeEvent{
					EventID: platform.NewEventID(), OrgID: orgID, ResourceID: result.ResourceID,
					RevisionID: result.RevisionID, Action: "graph.edges_upserted",
					Cursor: result.RevisionID + ":edges", OccurredAt: time.Now().UTC(),
				}); err != nil {
					return nil, platform.ErrUnavailable("change edge append failed: " + err.Error())
				}
			}
		}
		if s.Index != nil && result.ResourceID != "" {
			rec, err := s.Ledger.GetRecord(ctx, orgID, result.ResourceID)
			if err != nil {
				return nil, platform.ErrUnavailable("index load record failed: " + err.Error())
			}
			if rec.State != ports.LifecyclePlaceholder && rec.State != ports.LifecycleEnsured {
				if err := s.Index.Upsert(ctx, []ports.IndexDocument{{
					ResourceID: result.ResourceID, RevisionID: result.RevisionID, OrgID: orgID,
					Text: rec.Title, Labels: rec.Labels, Attributes: map[string]string{
						"classification": rec.Classification, "purpose_allowlist": "support",
					},
				}}); err != nil {
					return nil, platform.ErrUnavailable("index upsert failed: " + err.Error())
				}
			}
			seedMemoryAuthzResource(s.Authz, orgID, result.ResourceID, principal)
		}
	}

	return map[string]any{
		"event_id":          result.EventID,
		"organization_id":   result.OrganizationID,
		"resource_id":       result.ResourceID,
		"revision_id":       result.RevisionID,
		"status":            result.Status,
		"duplicate":         result.Duplicate,
		"idempotent_replay": result.IdempotentReplay,
	}, nil
}

// IntakeBatch accepts multiple CloudEvents and returns per-event results.
func (s *ApplicationService) IntakeBatch(ctx context.Context, creds ports.Credentials, orgID string, reqs []IntakeRequest) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0, len(reqs))
	accepted, rejected := 0, 0
	for _, req := range reqs {
		out, err := s.Intake(ctx, creds, orgID, req)
		if err != nil {
			rejected++
			ae, ok := platform.AsAPIError(err)
			item := map[string]any{"status": "rejected", "organization_id": orgID}
			if ok {
				item["reason_code"] = ae.ReasonCode
				item["message"] = ae.Message
			} else {
				item["message"] = err.Error()
			}
			if req.Event.EventID != "" {
				item["event_id"] = req.Event.EventID
			}
			results = append(results, item)
			continue
		}
		accepted++
		results = append(results, out)
	}
	return map[string]any{
		"results":         results,
		"accepted_count":  accepted,
		"rejected_count":  rejected,
	}, nil
}

func (s *ApplicationService) PresignEvidence(ctx context.Context, creds ports.Credentials, orgID, key, contentType string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	url, exp, err := s.Evidence.PresignPut(ctx, orgID+"/"+key, ports.PresignOptions{ContentType: contentType, ExpiresIn: 15 * time.Minute})
	if err != nil {
		return nil, err
	}
	return map[string]any{"upload_url": url, "expires_at": exp, "object_key": orgID + "/" + key}, nil
}

func (s *ApplicationService) ListChanges(ctx context.Context, creds ports.Credentials, orgID, cursor string, limit int) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	if s.ChangeFeed != nil {
		items, next, err := s.ChangeFeed.List(ctx, orgID, cursor, limit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	}
	if s.Changes == nil {
		return map[string]any{"items": []any{}, "next_cursor": cursor}, nil
	}
	items, next, err := s.Changes.ListChanges(ctx, orgID, cursor, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "next_cursor": next}, nil
}

func (s *ApplicationService) ManageWebhooks(ctx context.Context, creds ports.Credentials, orgID string, action string, body map[string]any) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	if s.ChangeFeed == nil {
		return map[string]any{"organization_id": orgID, "action": action, "status": "ok", "body": body}, nil
	}
	switch action {
	case "upsert", "create", "":
		target, _ := body["target_url"].(string)
		var events []string
		if raw, ok := body["events"].([]any); ok {
			for _, e := range raw {
				if s, ok := e.(string); ok {
					events = append(events, s)
				}
			}
		}
		secret, _ := body["secret"].(string)
		sub, err := s.ChangeFeed.UpsertWebhook(orgID, target, events, secret)
		if err != nil {
			return nil, err
		}
		// Best-effort test ping after register.
		_, _ = s.ChangeFeed.DeliverTestPing(ctx, nil, 3*time.Second, orgID, sub.ID)
		return map[string]any{"subscription": sub, "status": "ok"}, nil
	case "replay":
		subID, _ := body["subscription_id"].(string)
		eventID, _ := body["event_id"].(string)
		return s.ChangeFeed.Replay(ctx, orgID, subID, eventID)
	default:
		return map[string]any{"organization_id": orgID, "action": action, "status": "ok"}, nil
	}
}

func (s *ApplicationService) ListWebhooks(ctx context.Context, creds ports.Credentials, orgID string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	if s.ChangeFeed == nil {
		return map[string]any{"items": []any{}}, nil
	}
	return map[string]any{"items": s.ChangeFeed.ListWebhooks(orgID)}, nil
}

func (s *ApplicationService) GetWebhook(ctx context.Context, creds ports.Credentials, orgID, subscriptionID string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	if s.ChangeFeed == nil {
		return nil, platform.ErrNotFound("webhook subscription not found")
	}
	sub, err := s.ChangeFeed.GetWebhook(orgID, subscriptionID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"subscription": sub}, nil
}

func (s *ApplicationService) ListDeliveries(ctx context.Context, creds ports.Credentials, orgID, subscriptionID string, limit int) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	if s.ChangeFeed == nil {
		return map[string]any{"items": []any{}}, nil
	}
	return map[string]any{"items": s.ChangeFeed.ListDeliveries(orgID, subscriptionID, limit)}, nil
}

func (s *ApplicationService) StartExport(ctx context.Context, creds ports.Credentials, orgID string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	if s.Quota != nil {
		if err := s.Quota.Allow(quota.Key{OrgID: orgID, Op: quota.OpExport}); err != nil {
			return nil, err
		}
	}
	if s.Export == nil {
		id := platform.NewEventID()
		return map[string]any{"export_id": id, "status": "pending", "organization_id": orgID}, nil
	}
	manifest, err := s.Export.Build(ctx, orgID, principal.ID)
	if err != nil {
		return nil, err
	}
	if s.ExportJobs != nil {
		_ = s.ExportJobs.PutExportJob(ctx, orgID, manifest.ExportID, "completed", manifest)
	}
	return map[string]any{
		"export_id":       manifest.ExportID,
		"organization_id": orgID,
		"status":          "completed",
		"format_version":  manifest.FormatVersion,
		"created_at":      manifest.CreatedAt,
		"manifest":        manifest,
	}, nil
}

func (s *ApplicationService) GetExport(ctx context.Context, creds ports.Credentials, orgID, exportID string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	if s.ExportJobs == nil {
		return nil, platform.ErrNotFound("export not found")
	}
	id, status, manifest, err := s.ExportJobs.GetExportJob(ctx, orgID, exportID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"export_id": id, "organization_id": orgID, "status": status, "manifest": manifest,
	}, nil
}

func (s *ApplicationService) ImportExport(ctx context.Context, creds ports.Credentials, targetOrgID string, manifest export.Manifest) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, targetOrgID); err != nil {
		return nil, err
	}
	if s.Export == nil {
		return nil, platform.ErrUnavailable("export service not configured")
	}
	want := ""
	if manifest.Checksums != nil {
		want = manifest.Checksums["manifest_sha256"]
	}
	got := export.HashManifest(manifest)
	if want == "" || want != got {
		return nil, platform.ErrValidation("manifest hash verification failed")
	}
	// Import into isolated org: remap org id and re-seal checksum for the target.
	srcExportID := manifest.ExportID
	manifest.OrganizationID = targetOrgID
	manifest.Checksums = map[string]string{"manifest_sha256": export.HashManifest(manifest)}
	if err := s.Export.ImportInto(ctx, targetOrgID, manifest); err != nil {
		return nil, err
	}
	return map[string]any{
		"organization_id": targetOrgID,
		"status":          "imported",
		"export_id":       srcExportID,
		"source_sha256":   want,
		"manifest_sha256": manifest.Checksums["manifest_sha256"],
	}, nil
}

func (s *ApplicationService) DeleteResource(ctx context.Context, creds ports.Credentials, orgID, resourceID, reason string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	if s.Deletion == nil {
		return nil, platform.ErrUnavailable("deletion service not configured")
	}
	manifest, err := s.Deletion.Run(ctx, deletion.Request{
		OrgID: orgID, ResourceID: resourceID, Principal: principal, Reason: reason,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"manifest": manifest, "status": manifest.Status}, nil
}

func (s *ApplicationService) GetAudit(ctx context.Context, creds ports.Credentials, orgID string, limit int) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	if s.Extras == nil {
		return map[string]any{"items": []any{}}, nil
	}
	items, err := s.Extras.ListAudit(ctx, orgID, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items}, nil
}

func (s *ApplicationService) DiagnoseDecision(ctx context.Context, creds ports.Credentials, orgID, resourceID, action, purpose string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	dec, err := s.Authz.Check(ctx, ports.AuthzCheck{
		Principal: principal, Action: action, ResourceID: resourceID,
		Consistency: ports.ConsistencyFullyConsistent,
	})
	if err != nil {
		return nil, err
	}
	pol, err := s.Policy.Evaluate(ctx, ports.PolicyEval{
		Principal: principal, Action: action, Purpose: purpose, Classification: "internal",
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"organization_id": orgID,
		"authz":           dec,
		"policy":          pol,
	}, nil
}

func (s *ApplicationService) DiagnoseByAuditID(ctx context.Context, creds ports.Credentials, orgID, auditID string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	if s.Extras == nil {
		return nil, platform.ErrNotFound("audit event not found")
	}
	items, err := s.Extras.ListAudit(ctx, orgID, 500)
	if err != nil {
		return nil, err
	}
	for _, ev := range items {
		if ev.AuditID == auditID {
			resourceID := ""
			if len(ev.ResourceIDsSample) > 0 {
				resourceID = ev.ResourceIDsSample[0]
			}
			purpose := ""
			if ev.Attributes != nil {
				purpose = ev.Attributes["purpose"]
			}
			out, err := s.DiagnoseDecision(ctx, creds, orgID, resourceID, ev.Action, purpose)
			if err != nil {
				return nil, err
			}
			out["audit"] = ev
			return out, nil
		}
	}
	return nil, platform.ErrNotFound("audit event not found")
}

func (s *ApplicationService) GetQuotas(ctx context.Context, creds ports.Credentials, orgID string) (ports.Quota, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return ports.Quota{}, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return ports.Quota{}, err
	}
	if s.Quotas == nil {
		return ports.Quota{SearchPerMinute: 60, IntakePerMinute: 120, ExportPerMinute: 10, MaxResults: 25}, nil
	}
	return s.Quotas.GetQuotas(ctx, orgID)
}

func (s *ApplicationService) SetQuotas(ctx context.Context, creds ports.Credentials, orgID string, q ports.Quota) (ports.Quota, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return ports.Quota{}, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return ports.Quota{}, err
	}
	if s.Quotas == nil {
		return q, nil
	}
	if err := s.Quotas.SetQuotas(ctx, orgID, q); err != nil {
		return ports.Quota{}, err
	}
	return q, nil
}

func (s *ApplicationService) Bootstrap(ctx context.Context, creds ports.Credentials, orgID, name string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	org := ports.Organization{ID: orgID, Name: name, CreatedAt: time.Now().UTC()}
	if err := s.Ledger.CreateOrganization(ctx, org); err != nil {
		if !platform.IsAPIError(err) {
			return nil, err
		}
	}
	seedMemoryAuthzOrg(s.Authz, orgID, principal)
	return map[string]any{"organization_id": orgID, "name": name, "status": "ready"}, nil
}

func (s *ApplicationService) OrgStatus(ctx context.Context, creds ports.Credentials, orgID string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	org, err := s.Ledger.GetOrganization(ctx, orgID)
	if err != nil {
		return map[string]any{"organization_id": orgID, "status": "provisioning"}, nil
	}
	return map[string]any{
		"organization_id": org.ID,
		"name":            org.Name,
		"status":          "ready",
		"created_at":      org.CreatedAt,
	}, nil
}

func (s *ApplicationService) CreateAgent(ctx context.Context, creds ports.Credentials, orgID string, req ports.CreateAgentCredentialRequest) (ports.AgentCredential, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return ports.AgentCredential{}, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return ports.AgentCredential{}, err
	}
	if s.Credentials == nil {
		return ports.AgentCredential{}, platform.ErrUnavailable("credential provider not configured")
	}
	req.OrgID = orgID
	if req.AgentID == "" {
		req.AgentID = platform.NewEventID()
	}
	return s.Credentials.CreateAgentCredential(ctx, req)
}

func (s *ApplicationService) RotateAgent(ctx context.Context, creds ports.Credentials, orgID, agentID string) (ports.AgentCredential, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return ports.AgentCredential{}, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return ports.AgentCredential{}, err
	}
	if s.Credentials == nil {
		return ports.AgentCredential{}, platform.ErrUnavailable("credential provider not configured")
	}
	if rotator, ok := s.Credentials.(interface {
		RotateAgentCredential(context.Context, ports.CreateAgentCredentialRequest) (ports.AgentCredential, error)
	}); ok {
		return rotator.RotateAgentCredential(ctx, ports.CreateAgentCredentialRequest{OrgID: orgID, AgentID: agentID})
	}
	return s.Credentials.CreateAgentCredential(ctx, ports.CreateAgentCredentialRequest{OrgID: orgID, AgentID: agentID})
}

func (s *ApplicationService) RevokeAgent(ctx context.Context, creds ports.Credentials, orgID, credentialID string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	if s.Credentials == nil {
		return nil, platform.ErrUnavailable("credential provider not configured")
	}
	if err := s.Credentials.Revoke(ctx, orgID, credentialID); err != nil {
		return nil, err
	}
	return map[string]any{"organization_id": orgID, "credential_id": credentialID, "status": "revoked"}, nil
}

func (s *ApplicationService) OpsLag(ctx context.Context, creds ports.Credentials, orgID string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	pending, err := s.Ledger.ListOutboxPending(ctx, orgID, 1000)
	if err != nil {
		return nil, err
	}
	oldestAgeMs := int64(0)
	now := time.Now().UTC()
	for _, e := range pending {
		if e.CreatedAt.IsZero() {
			continue
		}
		age := now.Sub(e.CreatedAt).Milliseconds()
		if age > oldestAgeMs {
			oldestAgeMs = age
		}
	}
	return map[string]any{
		"organization_id":   orgID,
		"outbox_pending":    len(pending),
		"oldest_age_ms":     oldestAgeMs,
		"projection_lag_ms": 0,
		"status":            "ok",
		"note":              "projection_lag_ms reserved until durable projector metrics exist; outbox rows are not leased by this endpoint",
	}, nil
}

func (s *ApplicationService) SupportBundle(ctx context.Context, creds ports.Credentials, orgID string) (map[string]any, error) {
	principal, err := s.Identity.Authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return nil, err
	}
	v := s.Version()
	return map[string]any{
		"organization_id": orgID,
		"collected_at":    time.Now().UTC(),
		"version":         v,
		"health":          s.Health(),
		"notes":           "sanitized support bundle; no secrets or content",
	}, nil
}

func (s *ApplicationService) Health() map[string]any {
	ready, checks := s.ReadyStatus()
	status := "ok"
	if !ready {
		status = "degraded"
	}
	out := map[string]any{"status": status, "ready": ready}
	if checks != nil {
		out["checks"] = checks
	}
	return out
}

// ReadyStatus returns whether the process should accept traffic and the probe checks map.
func (s *ApplicationService) ReadyStatus() (bool, map[string]any) {
	if s.ReadyDetail != nil {
		return s.ReadyDetail()
	}
	ready := true
	if s.Ready != nil {
		ready = s.Ready()
	}
	checks := map[string]any{
		"migrations":         map[string]any{"ok": ready},
		"authz_model_pinned": map[string]any{"ok": strings.TrimSpace(s.Build.AuthzModelID) != ""},
	}
	if ready {
		authzOK := strings.TrimSpace(s.Build.AuthzModelID) != ""
		checks["authz_model_pinned"] = map[string]any{"ok": authzOK}
		if !authzOK {
			ready = false
		}
	}
	return ready, checks
}

func (s *ApplicationService) Version() VersionInfo {
	v := s.Build
	if v.APIVersion == "" {
		v.APIVersion = "1.0.0"
	}
	if v.PacketVersion == "" {
		v.PacketVersion = retrieval.PacketVersion
	}
	if v.ExportFormatVersion == "" {
		v.ExportFormatVersion = export.FormatVersion
	}
	if v.ProductVersion == "" {
		v.ProductVersion = "0.0.0-dev"
	}
	return v
}

func joinComma(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += "," + parts[i]
	}
	return out
}

func asRelationshipWriter(authz ports.AuthorizationProvider) ports.RelationshipWriter {
	if w, ok := authz.(ports.RelationshipWriter); ok {
		return w
	}
	return nil
}

// memoryAuthzSeeder is implemented by in-process OpenFGA (demo/tests).
type memoryAuthzSeeder interface {
	AddOrgMember(orgID, subject string)
	Grant(object, relation, subject string)
}

func principalSubject(p ports.Principal) string {
	if strings.TrimSpace(p.Subject) != "" {
		return strings.TrimSpace(p.Subject)
	}
	return strings.TrimSpace(p.ID)
}

func seedMemoryAuthzOrg(authz ports.AuthorizationProvider, orgID string, p ports.Principal) {
	m, ok := authz.(memoryAuthzSeeder)
	if !ok {
		return
	}
	subj := principalSubject(p)
	if subj == "" {
		return
	}
	m.AddOrgMember(orgID, subj)
	m.AddOrgMember(orgID, "user:"+subj)
	m.Grant("organization:"+orgID, "member", "user:"+subj)
	m.Grant("organization:"+orgID, "manager", "user:"+subj)
}

func seedMemoryAuthzResource(authz ports.AuthorizationProvider, orgID, resourceID string, p ports.Principal) {
	m, ok := authz.(memoryAuthzSeeder)
	if !ok || resourceID == "" {
		return
	}
	subj := principalSubject(p)
	if subj == "" {
		return
	}
	user := "user:" + subj
	m.AddOrgMember(orgID, subj)
	m.AddOrgMember(orgID, user)
	m.Grant("resource:"+resourceID, "organization", "organization:"+orgID)
	m.Grant("resource:"+resourceID, "owner", user)
	m.Grant("resource:"+resourceID, "reader", user)
}
