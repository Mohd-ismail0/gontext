package ports

import (
	"time"
)

// PrincipalKind classifies the authenticated actor.
type PrincipalKind string

const (
	PrincipalKindUser    PrincipalKind = "user"
	PrincipalKindAgent   PrincipalKind = "agent"
	PrincipalKindService PrincipalKind = "service"
	PrincipalKindGroup   PrincipalKind = "group"
)

// Principal is the normalized identity used by AuthZ/policy/audit.
type Principal struct {
	ID         string            `json:"id"`
	Kind       PrincipalKind     `json:"kind"`
	OrgID      string            `json:"org_id"`
	Issuer     string            `json:"issuer,omitempty"`
	Subject    string            `json:"subject"`
	Roles      []string          `json:"roles,omitempty"`
	Groups     []string          `json:"groups,omitempty"`
	Email      string            `json:"email,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// AgentPrincipal is an agent resolved from an API key / credential.
type AgentPrincipal struct {
	Principal
	AgentID      string `json:"agent_id"`
	CredentialID string `json:"credential_id"`
	OwnerID      string `json:"owner_id,omitempty"`
}

// DelegationGrant bounds an agent's authority to a subject's current rights.
type DelegationGrant struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"org_id"`
	SubjectID   string     `json:"subject_id"`
	ActorID     string     `json:"actor_id"`
	OwnerID     string     `json:"owner_id,omitempty"`
	Actions     []string   `json:"actions"`
	ResourceIDs []string   `json:"resource_ids,omitempty"`
	Purposes    []string   `json:"purposes,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Budget      *Quota     `json:"budget,omitempty"`
	Revoked     bool       `json:"revoked"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ConsistencyMode controls AuthZ read consistency (ADR 0006).
type ConsistencyMode string

const (
	ConsistencyMinLatency       ConsistencyMode = "min_latency"
	ConsistencyFullyConsistent  ConsistencyMode = "fully_consistent"
)

// Credentials wraps authentication material for IdentityProvider.
type Credentials struct {
	BearerToken string            `json:"bearer_token,omitempty"`
	APIKey      string            `json:"api_key,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// OIDCMetadata is discovery document subset used by clients.
type OIDCMetadata struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	JWKSURI               string   `json:"jwks_uri"`
	UserinfoEndpoint      string   `json:"userinfo_endpoint,omitempty"`
	ScopesSupported       []string `json:"scopes_supported,omitempty"`
}

// AuthzCheck is a single relationship authorization request.
type AuthzCheck struct {
	Principal   Principal       `json:"principal"`
	Action      string          `json:"action"`
	ResourceID  string          `json:"resource_id"`
	Consistency ConsistencyMode `json:"consistency"`
	Context     map[string]any  `json:"context,omitempty"`
	Delegation  *DelegationGrant `json:"delegation,omitempty"`
}

// AuthzDecision is the AuthZ allow/deny outcome (not disclosure policy).
type AuthzDecision struct {
	Allowed        bool            `json:"allowed"`
	ReasonCode     string          `json:"reason_code,omitempty"`
	Consistency    ConsistencyMode `json:"consistency"`
	ModelRevision  string          `json:"model_revision,omitempty"`
	CheckedAt      time.Time       `json:"checked_at"`
}

// ScopeResolve asks AuthZ for a candidate prefilter scope.
type ScopeResolve struct {
	Principal   Principal       `json:"principal"`
	Action      string          `json:"action"`
	Consistency ConsistencyMode `json:"consistency"`
	Filters     map[string]any  `json:"filters,omitempty"`
}

// CandidateScope is a safe prefilter for retrieval (never a grant).
type CandidateScope struct {
	OrgID       string   `json:"org_id"`
	ResourceIDs []string `json:"resource_ids,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	ReasonCode  string   `json:"reason_code,omitempty"`
}

// PolicyEval evaluates disclosure/redaction/purpose obligations after AuthZ allow.
type PolicyEval struct {
	Principal      Principal `json:"principal"`
	Action         string    `json:"action"`
	Purpose        string    `json:"purpose"`
	Classification string    `json:"classification"`
	Record         *Record   `json:"record,omitempty"`
	RequestedLimit int       `json:"requested_limit,omitempty"`
}

// PolicyResult carries obligations only; it never replaces AuthZ allow.
type PolicyResult struct {
	Allow            bool     `json:"allow"`
	RedactionProfile string   `json:"redaction_profile"`
	Obligations      []string `json:"obligations,omitempty"`
	MaxResults       int      `json:"max_results,omitempty"`
	ReasonCode       string   `json:"reason_code,omitempty"`
}

// Record is a canonical ledger resource (stable logical entity).
type Record struct {
	ResourceID     string            `json:"resource_id"`
	OrgID          string            `json:"org_id"`
	Kind           string            `json:"kind"`
	Title          string            `json:"title,omitempty"`
	Classification string            `json:"classification,omitempty"`
	Labels         []string          `json:"labels,omitempty"`
	SourceSystem   string            `json:"source_system,omitempty"`
	ExternalID     string            `json:"external_id,omitempty"`
	CurrentRevID   string            `json:"current_revision_id,omitempty"`
	State          string            `json:"state,omitempty"`
	Attributes     map[string]string `json:"attributes,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// Revision is an immutable observed version of a Record.
type Revision struct {
	RevisionID   string            `json:"revision_id"`
	ResourceID   string            `json:"resource_id"`
	OrgID        string            `json:"org_id"`
	ContentHash  string            `json:"content_hash,omitempty"`
	EvidenceRef  string            `json:"evidence_ref,omitempty"`
	Sequence     int64             `json:"sequence"`
	State        string            `json:"state"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	ObservedAt   time.Time         `json:"observed_at"`
	CreatedAt    time.Time         `json:"created_at"`
}

// ContextPacket is a governed retrieval response bundle.
type ContextPacket struct {
	Version            string      `json:"version"`
	PacketID           string      `json:"packet_id,omitempty"`
	OrgID              string      `json:"organization_id"`
	Purpose            string      `json:"purpose"`
	RedactionProfile   string      `json:"redaction_profile,omitempty"`
	Citations          []Citation  `json:"citations"`
	Redactions          []Redaction `json:"redactions"`
	Summary            string      `json:"summary,omitempty"`
	PolicyRevision     string      `json:"policy_revision"`
	AuthzRevision      string      `json:"authz_revision"`
	AuditID            string      `json:"audit_id"`
	ActionRestrictions []string    `json:"action_restrictions"`
	GeneratedAt        time.Time   `json:"responded_at,omitempty"`
}

// Redaction records an applied disclosure obligation.
type Redaction struct {
	Profile    string   `json:"profile"`
	ReasonCode string   `json:"reason_code"`
	Fields     []string `json:"fields,omitempty"`
}

// Citation points at a specific revision/snippet used in a packet.
type Citation struct {
	CitationID string  `json:"citation_id,omitempty"`
	ResourceID string  `json:"resource_id"`
	RevisionID string  `json:"revision_id"`
	Snippet    string  `json:"snippet,omitempty"`
	Score      float64 `json:"score,omitempty"`
}

// SearchRequest is the retrieval query shape (never logged verbatim in audit).
type SearchRequest struct {
	OrgID          string            `json:"org_id"`
	Query          string            `json:"query,omitempty"`
	QueryHash      string            `json:"query_hash,omitempty"`
	Purpose        string            `json:"purpose"`
	Limit          int               `json:"limit"`
	Filters        map[string]string `json:"filters,omitempty"`
	Consistency    ConsistencyMode   `json:"consistency"`
	IncludeVectors bool              `json:"include_vectors,omitempty"`
	Scopes         []string          `json:"scopes,omitempty"`
}

// IntakeEvent is a validated CloudEvents-shaped intake payload reference.
type IntakeEvent struct {
	EventID        string            `json:"event_id"`
	OrgID          string            `json:"org_id"`
	SourceSystem   string            `json:"source_system"`
	ExternalID     string            `json:"external_id"`
	SourceRevision string            `json:"source_revision"`
	IdempotencyKey string            `json:"idempotency_key"`
	ContentType    string            `json:"content_type"`
	ContentHash    string            `json:"content_hash"`
	EvidenceRef    string            `json:"evidence_ref,omitempty"`
	Attributes     map[string]string `json:"attributes,omitempty"`
	ReceivedAt     time.Time         `json:"received_at"`
}

// SourceRegistration describes a registered source/connector binding.
type SourceRegistration struct {
	SourceID               string            `json:"source_id"`
	OrgID                  string            `json:"organization_id"`
	System                 string            `json:"system"`
	DisplayName            string            `json:"display_name,omitempty"`
	TrustTier             string            `json:"trust_tier,omitempty"`
	TrustCeiling           string            `json:"trust_ceiling,omitempty"`
	AuthorityCeiling       string            `json:"authority_ceiling,omitempty"`
	ClassificationCeiling  string            `json:"classification_ceiling,omitempty"`
	ClassificationDefault  string            `json:"classification_default,omitempty"`
	MappingSpec            string            `json:"mapping_spec_id,omitempty"`
	Enabled                bool              `json:"enabled"`
	SigningSecret          string            `json:"signing_secret,omitempty"`
	ReplayWindowSeconds    int               `json:"replay_window_seconds,omitempty"`
	AllowedRecordTypes     []string          `json:"allowed_record_types,omitempty"`
	AllowedVisibilityRefs  []string          `json:"allowed_visibility_refs,omitempty"`
	Attributes             map[string]string `json:"attributes,omitempty"`
	CreatedAt              time.Time         `json:"created_at"`
	UpdatedAt              time.Time         `json:"updated_at"`
}

// EffectiveTrustCeiling returns the trust ceiling (prefer explicit ceiling).
func (s SourceRegistration) EffectiveTrustCeiling() string {
	if s.TrustCeiling != "" {
		return s.TrustCeiling
	}
	return s.TrustTier
}

// EffectiveClassificationCeiling returns classification ceiling or default.
func (s SourceRegistration) EffectiveClassificationCeiling() string {
	if s.ClassificationCeiling != "" {
		return s.ClassificationCeiling
	}
	if s.ClassificationDefault != "" {
		return s.ClassificationDefault
	}
	return "internal"
}

// MappingSpec transforms source payloads into canonical intake fields.
type MappingSpec struct {
	ID         string            `json:"id"`
	OrgID      string            `json:"organization_id"`
	Version    string            `json:"version"`
	SourceKind string            `json:"source_kind"`
	SourceID   string            `json:"source_id,omitempty"`
	Rules      map[string]string `json:"rules"`
	CreatedAt  time.Time         `json:"created_at"`
}

// IdempotencyRecord stores the outcome of a prior intake for UNIQUE key replay.
type IdempotencyRecord struct {
	OrgID          string    `json:"organization_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	EventID        string    `json:"event_id"`
	ResourceID     string    `json:"resource_id"`
	RevisionID     string    `json:"revision_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// ChangeEvent is a metadata-only change feed item.
type ChangeEvent struct {
	EventID    string    `json:"event_id"`
	OrgID      string    `json:"org_id"`
	ResourceID string    `json:"resource_id"`
	RevisionID string    `json:"revision_id,omitempty"`
	Action     string    `json:"action"`
	Cursor     string    `json:"cursor"`
	OccurredAt time.Time `json:"occurred_at"`
}

// Quota captures budget/rate limits for an org, source, or principal.
type Quota struct {
	SearchPerMinute int `json:"search_per_minute,omitempty"`
	IntakePerMinute int `json:"intake_per_minute,omitempty"`
	ExportPerMinute int `json:"export_per_minute,omitempty"`
	MaxResults      int `json:"max_results,omitempty"`
}

// AuditEvent is a sanitized, reconstructable disclosure/action record.
// Never store raw queries, content, or tokens.
type AuditEvent struct {
	AuditID           string            `json:"audit_id"`
	OrgID             string            `json:"org_id"`
	PrincipalID       string            `json:"principal_id"`
	PrincipalKind     PrincipalKind     `json:"principal_kind"`
	DelegationID      string            `json:"delegation_id,omitempty"`
	Action            string            `json:"action"`
	ReasonCode        string            `json:"reason_code"`
	AuthzModelRev     string            `json:"authz_model_revision,omitempty"`
	PolicyRevision    string            `json:"policy_revision,omitempty"`
	ResourceCount     int               `json:"resource_count,omitempty"`
	ResourceIDsSample []string          `json:"resource_ids_sample,omitempty"`
	TraceID           string            `json:"trace_id,omitempty"`
	Attributes        map[string]string `json:"attributes,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
}

// CreateAgentCredentialRequest creates a new agent API credential.
type CreateAgentCredentialRequest struct {
	OrgID        string     `json:"org_id"`
	AgentID      string     `json:"agent_id"`
	OwnerID      string     `json:"owner_id"`
	DelegationID string     `json:"delegation_id,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Label        string     `json:"label,omitempty"`
}

// AgentCredential is the issued credential metadata (secret shown once).
type AgentCredential struct {
	CredentialID string     `json:"credential_id"`
	AgentID      string     `json:"agent_id"`
	OrgID        string     `json:"org_id"`
	Secret       string     `json:"secret,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// EventMessage is a bus message delivered to subscribers.
type EventMessage struct {
	Subject   string            `json:"subject"`
	Data      []byte            `json:"data"`
	Headers   map[string]string `json:"headers,omitempty"`
	MsgID     string            `json:"msg_id,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// Subscription is a cancellable event bus subscription.
type Subscription interface {
	Unsubscribe() error
}

// EvidenceObject metadata for stored evidence blobs.
type EvidenceObject struct {
	Key         string            `json:"key"`
	VersionID   string            `json:"version_id,omitempty"`
	ContentHash string            `json:"content_hash,omitempty"`
	Size        int64             `json:"size"`
	ContentType string            `json:"content_type,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// PresignOptions configures a time-limited put URL.
type PresignOptions struct {
	ContentType string
	ExpiresIn   time.Duration
	Metadata    map[string]string
}

// IndexDocument is a rebuildable projection document.
type IndexDocument struct {
	ResourceID string            `json:"resource_id"`
	RevisionID string            `json:"revision_id"`
	OrgID      string            `json:"org_id"`
	Text       string            `json:"text,omitempty"`
	Embedding  []float32         `json:"embedding,omitempty"`
	Labels     []string          `json:"labels,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// SearchHit is a candidate from IndexProvider (pre-AuthZ filter).
type SearchHit struct {
	ResourceID string  `json:"resource_id"`
	RevisionID string  `json:"revision_id"`
	Score      float64 `json:"score"`
}

// ParseRequest asks a ParserProvider to extract text from evidence.
type ParseRequest struct {
	OrgID       string
	EvidenceRef string
	ContentType string
	MaxBytes    int64
}

// ParseResult is extracted text plus confidence.
type ParseResult struct {
	Text       string
	Confidence float64
	Pages      int
	Warnings   []string
}

// Organization is a tenant root.
type Organization struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Attributes map[string]string `json:"attributes,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// OutboxEntry is a transactional outbox row.
type OutboxEntry struct {
	ID        string            `json:"id"`
	OrgID     string            `json:"org_id"`
	Subject   string            `json:"subject"`
	Payload   []byte            `json:"payload"`
	Headers   map[string]string `json:"headers,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	Published *time.Time        `json:"published_at,omitempty"`
}

// InboxEntry tracks idempotent consumer processing.
type InboxEntry struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Consumer    string    `json:"consumer"`
	MsgID       string    `json:"msg_id"`
	ProcessedAt time.Time `json:"processed_at"`
}
