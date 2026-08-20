package ports

import (
	"context"
	"io"
	"time"
)

// IdentityProvider authenticates principals and discovers OIDC metadata.
type IdentityProvider interface {
	Authenticate(ctx context.Context, credentials Credentials) (Principal, error)
	Discover(ctx context.Context) (OIDCMetadata, error)
}

// AuthorizationProvider performs relationship AuthZ checks (OpenFGA, etc.).
type AuthorizationProvider interface {
	Check(ctx context.Context, req AuthzCheck) (AuthzDecision, error)
	BatchCheck(ctx context.Context, reqs []AuthzCheck) ([]AuthzDecision, error)
	ResolveCandidateScope(ctx context.Context, req ScopeResolve) (CandidateScope, error)
}

// PolicyProvider applies deterministic disclosure/redaction/purpose obligations.
// It never grants access; callers must already have an AuthZ allow.
type PolicyProvider interface {
	Evaluate(ctx context.Context, req PolicyEval) (PolicyResult, error)
}

// CredentialProvider resolves and manages agent API credentials.
type CredentialProvider interface {
	ResolveAPIKey(ctx context.Context, key string) (AgentPrincipal, error)
	CreateAgentCredential(ctx context.Context, req CreateAgentCredentialRequest) (AgentCredential, error)
	Revoke(ctx context.Context, credentialID string) error
}

// EventBus publishes and subscribes to durable domain events.
type EventBus interface {
	Publish(ctx context.Context, subject string, data []byte, headers map[string]string) error
	Subscribe(ctx context.Context, subject string, handler func(msg EventMessage) error) (Subscription, error)
}

// EvidenceStore is the versioned object store for raw/derived evidence.
type EvidenceStore interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string, meta map[string]string) (EvidenceObject, error)
	Get(ctx context.Context, key, versionID string) (io.ReadCloser, EvidenceObject, error)
	PresignPut(ctx context.Context, key string, opts PresignOptions) (string, time.Time, error)
	DeleteVersion(ctx context.Context, key, versionID string) error
}

// IndexProvider is a rebuildable search/vector projection.
type IndexProvider interface {
	SearchCandidates(ctx context.Context, orgID string, query string, limit int, filters map[string]string) ([]SearchHit, error)
	Upsert(ctx context.Context, docs []IndexDocument) error
	Delete(ctx context.Context, orgID string, resourceIDs []string) error
}

// ParserProvider extracts text from evidence blobs.
type ParserProvider interface {
	Parse(ctx context.Context, req ParseRequest) (ParseResult, error)
}

// EmbeddingProvider produces dense vectors for projection.
type EmbeddingProvider interface {
	Embed(ctx context.Context, orgID string, texts []string) ([][]float32, error)
}

// Tx is a tenant-scoped ledger transaction with RLS applied.
type Tx interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// LedgerStore is the authoritative PostgreSQL/RLS ledger.
type LedgerStore interface {
	// WithOrgTx runs fn inside a transaction with org RLS context set.
	WithOrgTx(ctx context.Context, orgID string, fn func(ctx context.Context, tx Tx) error) error

	CreateOrganization(ctx context.Context, org Organization) error
	GetOrganization(ctx context.Context, orgID string) (Organization, error)

	UpsertRecord(ctx context.Context, tx Tx, rec Record) error
	GetRecord(ctx context.Context, orgID, resourceID string) (Record, error)
	ListRecords(ctx context.Context, orgID string, limit int, cursor string) ([]Record, string, error)

	AppendRevision(ctx context.Context, tx Tx, rev Revision) error
	GetRevision(ctx context.Context, orgID, revisionID string) (Revision, error)
	ListRevisions(ctx context.Context, orgID, resourceID string, limit int) ([]Revision, error)

	EnqueueOutbox(ctx context.Context, tx Tx, entry OutboxEntry) error
	// ClaimOutbox leases unpublished outbox rows for relay (at-least-once).
	ClaimOutbox(ctx context.Context, limit int) ([]OutboxEntry, error)
	MarkOutboxPublished(ctx context.Context, ids []string, at time.Time) error
	// ListOutboxPending returns unpublished outbox rows without leasing them.
	ListOutboxPending(ctx context.Context, orgID string, limit int) ([]OutboxEntry, error)

	PutInbox(ctx context.Context, entry InboxEntry) error
	HasInbox(ctx context.Context, orgID, consumer, msgID string) (bool, error)

	GetIdempotency(ctx context.Context, orgID, key string) (IdempotencyRecord, error)
	PutIdempotency(ctx context.Context, tx Tx, rec IdempotencyRecord) error

	UpsertSource(ctx context.Context, tx Tx, src SourceRegistration) error
	GetSource(ctx context.Context, orgID, sourceID string) (SourceRegistration, error)
	ListSources(ctx context.Context, orgID string) ([]SourceRegistration, error)

	CreateDelegation(ctx context.Context, tx Tx, grant DelegationGrant) error
	GetDelegation(ctx context.Context, orgID, grantID string) (DelegationGrant, error)
	RevokeDelegation(ctx context.Context, tx Tx, orgID, grantID string) error
	ListDelegations(ctx context.Context, orgID, actorID string) ([]DelegationGrant, error)

	// Knowledge-graph edges (ADR 0013). AuthZ is separate (OpenFGA).
	UpsertEdge(ctx context.Context, tx Tx, edge GraphEdge) error
	GetEdge(ctx context.Context, orgID, edgeID string) (GraphEdge, error)
	ListEdges(ctx context.Context, orgID string, opts EdgeListOptions) ([]GraphEdge, error)
	TombstoneEdge(ctx context.Context, tx Tx, orgID, edgeID string) error
}

// EdgeListOptions filters knowledge-graph edge queries.
type EdgeListOptions struct {
	// ResourceIDs matches edges where from_id OR to_id is in the set.
	ResourceIDs []string
	Predicates  []string
	IncludeDead bool // include TOMBSTONED
	Limit       int
}
