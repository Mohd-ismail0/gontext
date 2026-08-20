package memory

import (
	"context"
	"sync"
	"time"

	"github.com/xsama/context-fabric/internal/mapping"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// Store is an in-memory LedgerStore with org-keyed maps (tenant isolation).
type Store struct {
	mu           sync.RWMutex
	orgs         map[string]ports.Organization
	records      map[string]map[string]ports.Record // org -> resourceID -> record
	revisions    map[string]map[string]ports.Revision
	sources      map[string]map[string]ports.SourceRegistration
	delegations  map[string]map[string]ports.DelegationGrant
	outbox       []ports.OutboxEntry
	outboxLease  map[string]time.Time // outbox id -> lease expiry
	inbox        map[string]ports.InboxEntry // org|consumer|msgID
	idempotency  map[string]ports.IdempotencyRecord // org|key
	changes      map[string][]ports.ChangeEvent
	audits       map[string][]ports.AuditEvent
	quotas       map[string]ports.Quota
	webhooks     map[string]map[string]WebhookSubscription
	accessReqs   map[string]map[string]AccessRequest
	exports      map[string]map[string]ExportJob
	holds        map[string]map[string]bool // org -> resourceID -> legal hold
	mappings     map[string]mapping.Spec    // sourceID -> Spec
	edges        map[string]map[string]ports.GraphEdge // org -> edgeID -> edge
	authzTuples  map[string]map[string]ports.AuthzTupleOp // org -> id -> op
}

// WebhookSubscription is a test/demo webhook registration.
type WebhookSubscription struct {
	ID        string
	OrgID     string
	TargetURL string
	Events    []string
	Enabled   bool
	CreatedAt time.Time
}

// AccessRequest tracks a pending access request.
type AccessRequest struct {
	RequestID     string
	OrgID         string
	ResourceID    string
	Purpose       string
	Justification string
	Status        string
	CreatedAt     time.Time
	AuditID       string
}

// ExportJob tracks an export.
type ExportJob struct {
	JobID     string
	OrgID     string
	Status    string
	CreatedAt time.Time
	Manifest  any
}

// NewStore creates an empty in-memory ledger.
func NewStore() *Store {
	return &Store{
		orgs:        make(map[string]ports.Organization),
		records:     make(map[string]map[string]ports.Record),
		revisions:   make(map[string]map[string]ports.Revision),
		sources:     make(map[string]map[string]ports.SourceRegistration),
		delegations: make(map[string]map[string]ports.DelegationGrant),
		outboxLease: make(map[string]time.Time),
		inbox:       make(map[string]ports.InboxEntry),
		idempotency: make(map[string]ports.IdempotencyRecord),
		changes:     make(map[string][]ports.ChangeEvent),
		audits:      make(map[string][]ports.AuditEvent),
		quotas:      make(map[string]ports.Quota),
		webhooks:    make(map[string]map[string]WebhookSubscription),
		accessReqs:  make(map[string]map[string]AccessRequest),
		exports:     make(map[string]map[string]ExportJob),
		holds:       make(map[string]map[string]bool),
		mappings:    make(map[string]mapping.Spec),
		edges:       make(map[string]map[string]ports.GraphEdge),
		authzTuples: make(map[string]map[string]ports.AuthzTupleOp),
	}
}

var _ ports.LedgerStore = (*Store)(nil)

type memTx struct{ committed, rolled bool }

func (t *memTx) Commit(context.Context) error   { t.committed = true; return nil }
func (t *memTx) Rollback(context.Context) error { t.rolled = true; return nil }

// WithOrgTx runs fn with a no-op transaction (tenant isolation via map keys).
func (s *Store) WithOrgTx(ctx context.Context, orgID string, fn func(ctx context.Context, tx ports.Tx) error) error {
	if orgID == "" {
		return platform.ErrValidation("organization_id required")
	}
	tx := &memTx{}
	if err := fn(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// WithTenant mirrors the postgres helper for shared call sites.
func (s *Store) WithTenant(ctx context.Context, orgID string, fn func(ctx context.Context) error) error {
	return s.WithOrgTx(ctx, orgID, func(ctx context.Context, _ ports.Tx) error {
		return fn(ctx)
	})
}

func (s *Store) CreateOrganization(_ context.Context, org ports.Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orgs[org.ID]; ok {
		return platform.ErrConflict("organization exists")
	}
	if org.CreatedAt.IsZero() {
		org.CreatedAt = time.Now().UTC()
	}
	s.orgs[org.ID] = org
	return nil
}

func (s *Store) GetOrganization(_ context.Context, orgID string) (ports.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o, ok := s.orgs[orgID]
	if !ok {
		return ports.Organization{}, platform.ErrNotFound("organization not found")
	}
	return o, nil
}

func (s *Store) UpsertRecord(_ context.Context, _ ports.Tx, rec ports.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec.OrgID == "" || rec.ResourceID == "" {
		return platform.ErrValidation("org and resource required")
	}
	m := s.records[rec.OrgID]
	if m == nil {
		m = make(map[string]ports.Record)
		s.records[rec.OrgID] = m
	}
	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now
	m[rec.ResourceID] = rec
	return nil
}

func (s *Store) GetRecord(_ context.Context, orgID, resourceID string) (ports.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.records[orgID]
	if m == nil {
		return ports.Record{}, platform.ErrNotFound("record not found")
	}
	r, ok := m[resourceID]
	if !ok {
		return ports.Record{}, platform.ErrNotFound("record not found")
	}
	return r, nil
}

func (s *Store) ListRecords(_ context.Context, orgID string, limit int, _ string) ([]ports.Record, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	out := make([]ports.Record, 0, limit)
	for _, r := range s.records[orgID] {
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out, "", nil
}

func (s *Store) AppendRevision(_ context.Context, _ ports.Tx, rev ports.Revision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.revisions[rev.OrgID]
	if m == nil {
		m = make(map[string]ports.Revision)
		s.revisions[rev.OrgID] = m
	}
	if rev.CreatedAt.IsZero() {
		rev.CreatedAt = time.Now().UTC()
	}
	m[rev.RevisionID] = rev
	return nil
}

func (s *Store) GetRevision(_ context.Context, orgID, revisionID string) (ports.Revision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.revisions[orgID]
	if m == nil {
		return ports.Revision{}, platform.ErrNotFound("revision not found")
	}
	r, ok := m[revisionID]
	if !ok {
		return ports.Revision{}, platform.ErrNotFound("revision not found")
	}
	return r, nil
}

func (s *Store) ListRevisions(_ context.Context, orgID, resourceID string, limit int) ([]ports.Revision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	out := make([]ports.Revision, 0)
	for _, r := range s.revisions[orgID] {
		if r.ResourceID == resourceID {
			out = append(out, r)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (s *Store) EnqueueOutbox(_ context.Context, _ ports.Tx, entry ports.OutboxEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		entry.ID = platform.NewEventID()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	s.outbox = append(s.outbox, entry)
	return nil
}

func (s *Store) ClaimOutbox(_ context.Context, limit int) ([]ports.OutboxEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 10
	}
	now := time.Now().UTC()
	leaseUntil := now.Add(30 * time.Second)
	var out []ports.OutboxEntry
	for _, e := range s.outbox {
		if e.Published != nil {
			continue
		}
		if until, ok := s.outboxLease[e.ID]; ok && until.After(now) {
			continue
		}
		s.outboxLease[e.ID] = leaseUntil
		if e.Headers == nil {
			e.Headers = map[string]string{}
		}
		e.Headers["leased_at"] = now.Format(time.RFC3339Nano)
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ListOutboxPending returns unpublished outbox rows without leasing/claiming them.
func (s *Store) ListOutboxPending(_ context.Context, orgID string, limit int) ([]ports.OutboxEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	var out []ports.OutboxEntry
	for _, e := range s.outbox {
		if e.Published != nil {
			continue
		}
		if orgID != "" && e.OrgID != orgID {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// PutMappingSpec stores a MappingSpec keyed by sourceID.
func (s *Store) PutMappingSpec(_ context.Context, orgID, sourceID string, spec mapping.Spec) error {
	if sourceID == "" {
		return platform.ErrValidation("source_id required")
	}
	if spec.OrganizationID == "" {
		spec.OrganizationID = orgID
	}
	if spec.SourceID == "" {
		spec.SourceID = sourceID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mappings[sourceID] = spec
	return nil
}

// GetMappingSpec loads a MappingSpec by sourceID.
func (s *Store) GetMappingSpec(_ context.Context, orgID, sourceID string) (mapping.Spec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	spec, ok := s.mappings[sourceID]
	if !ok {
		return mapping.Spec{}, platform.ErrNotFound("mapping spec not found")
	}
	if orgID != "" && spec.OrganizationID != "" && spec.OrganizationID != orgID {
		return mapping.Spec{}, platform.ErrNotFound("mapping spec not found")
	}
	return spec, nil
}

// PutAccessRequest persists a pending access request with justification.
func (s *Store) PutAccessRequest(_ context.Context, orgID, requestID, resourceID, purpose, justification, auditID string) error {
	if orgID == "" || requestID == "" {
		return platform.ErrValidation("org and request_id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.accessReqs[orgID]
	if m == nil {
		m = make(map[string]AccessRequest)
		s.accessReqs[orgID] = m
	}
	m[requestID] = AccessRequest{
		RequestID: requestID, OrgID: orgID, ResourceID: resourceID,
		Purpose: purpose, Justification: justification, Status: "pending",
		CreatedAt: time.Now().UTC(), AuditID: auditID,
	}
	return nil
}

// GetAccessRequest returns a stored access request.
func (s *Store) GetAccessRequest(_ context.Context, orgID, requestID string) (AccessRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.accessReqs[orgID]
	if m == nil {
		return AccessRequest{}, platform.ErrNotFound("access request not found")
	}
	req, ok := m[requestID]
	if !ok {
		return AccessRequest{}, platform.ErrNotFound("access request not found")
	}
	return req, nil
}

func (s *Store) GetIdempotency(_ context.Context, orgID, key string) (ports.IdempotencyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.idempotency[orgID+"|"+key]
	if !ok {
		return ports.IdempotencyRecord{}, platform.ErrNotFound("idempotency key not found")
	}
	return rec, nil
}

func (s *Store) PutIdempotency(_ context.Context, _ ports.Tx, rec ports.IdempotencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := rec.OrgID + "|" + rec.IdempotencyKey
	if _, exists := s.idempotency[key]; exists {
		return platform.ErrConflict("idempotency key exists")
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	s.idempotency[key] = rec
	return nil
}

func (s *Store) MarkOutboxPublished(_ context.Context, ids []string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	set := map[string]struct{}{}
	for _, id := range ids {
		set[id] = struct{}{}
		delete(s.outboxLease, id)
	}
	for i := range s.outbox {
		if _, ok := set[s.outbox[i].ID]; ok {
			t := at
			s.outbox[i].Published = &t
		}
	}
	return nil
}

func (s *Store) PutInbox(_ context.Context, entry ports.InboxEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := entry.OrgID + "|" + entry.Consumer + "|" + entry.MsgID
	s.inbox[key] = entry
	return nil
}

func (s *Store) HasInbox(_ context.Context, orgID, consumer, msgID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.inbox[orgID+"|"+consumer+"|"+msgID]
	return ok, nil
}

func (s *Store) UpsertSource(_ context.Context, _ ports.Tx, src ports.SourceRegistration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.sources[src.OrgID]
	if m == nil {
		m = make(map[string]ports.SourceRegistration)
		s.sources[src.OrgID] = m
	}
	now := time.Now().UTC()
	if src.CreatedAt.IsZero() {
		src.CreatedAt = now
	}
	src.UpdatedAt = now
	src.MappingSpecInline = nil
	m[src.SourceID] = src
	return nil
}

func (s *Store) GetSource(_ context.Context, orgID, sourceID string) (ports.SourceRegistration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.sources[orgID]
	if m == nil {
		return ports.SourceRegistration{}, platform.ErrNotFound("source not found")
	}
	src, ok := m[sourceID]
	if !ok {
		return ports.SourceRegistration{}, platform.ErrNotFound("source not found")
	}
	return src, nil
}

func (s *Store) ListSources(_ context.Context, orgID string) ([]ports.SourceRegistration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ports.SourceRegistration, 0, len(s.sources[orgID]))
	for _, src := range s.sources[orgID] {
		out = append(out, src)
	}
	return out, nil
}

func (s *Store) CreateDelegation(_ context.Context, _ ports.Tx, grant ports.DelegationGrant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.delegations[grant.OrgID]
	if m == nil {
		m = make(map[string]ports.DelegationGrant)
		s.delegations[grant.OrgID] = m
	}
	if grant.CreatedAt.IsZero() {
		grant.CreatedAt = time.Now().UTC()
	}
	m[grant.ID] = grant
	return nil
}

func (s *Store) GetDelegation(_ context.Context, orgID, grantID string) (ports.DelegationGrant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.delegations[orgID]
	if m == nil {
		return ports.DelegationGrant{}, platform.ErrNotFound("delegation not found")
	}
	g, ok := m[grantID]
	if !ok {
		return ports.DelegationGrant{}, platform.ErrNotFound("delegation not found")
	}
	return g, nil
}

func (s *Store) RevokeDelegation(_ context.Context, _ ports.Tx, orgID, grantID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.delegations[orgID]
	if m == nil {
		return platform.ErrNotFound("delegation not found")
	}
	g, ok := m[grantID]
	if !ok {
		return platform.ErrNotFound("delegation not found")
	}
	g.Revoked = true
	m[grantID] = g
	return nil
}

func (s *Store) ListDelegations(_ context.Context, orgID, actorID string) ([]ports.DelegationGrant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ports.DelegationGrant, 0)
	for _, g := range s.delegations[orgID] {
		if actorID == "" || g.ActorID == actorID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (s *Store) UpsertEdge(_ context.Context, _ ports.Tx, edge ports.GraphEdge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if edge.OrgID == "" || edge.FromID == "" || edge.ToID == "" || edge.Predicate == "" {
		return platform.ErrValidation("org, from_id, to_id, predicate required")
	}
	if edge.FromID == edge.ToID {
		return platform.ErrValidation("self-edges are not allowed")
	}
	if edge.EdgeID == "" {
		edge.EdgeID = platform.NewEventID()
	}
	now := time.Now().UTC()
	if edge.CreatedAt.IsZero() {
		edge.CreatedAt = now
	}
	edge.UpdatedAt = now
	if edge.State == "" {
		edge.State = "ACTIVE"
	}
	m := s.edges[edge.OrgID]
	if m == nil {
		m = make(map[string]ports.GraphEdge)
		s.edges[edge.OrgID] = m
	}
	// Idempotent: if identical active triple exists, refresh metadata in place.
	for id, existing := range m {
		if existing.State == "ACTIVE" &&
			existing.FromID == edge.FromID &&
			existing.ToID == edge.ToID &&
			existing.Predicate == edge.Predicate {
			existing.Confidence = edge.Confidence
			existing.SyncAuthz = edge.SyncAuthz
			existing.Attributes = edge.Attributes
			existing.UpdatedAt = now
			m[id] = existing
			return nil
		}
	}
	m[edge.EdgeID] = edge
	return nil
}

func (s *Store) GetEdge(_ context.Context, orgID, edgeID string) (ports.GraphEdge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.edges[orgID]
	if m == nil {
		return ports.GraphEdge{}, platform.ErrNotFound("edge not found")
	}
	e, ok := m[edgeID]
	if !ok {
		return ports.GraphEdge{}, platform.ErrNotFound("edge not found")
	}
	return e, nil
}

func (s *Store) ListEdges(_ context.Context, orgID string, opts ports.EdgeListOptions) ([]ports.GraphEdge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if opts.Limit <= 0 {
		opts.Limit = 200
	}
	wantRes := map[string]bool{}
	for _, id := range opts.ResourceIDs {
		wantRes[id] = true
	}
	wantPred := map[string]bool{}
	for _, p := range opts.Predicates {
		wantPred[p] = true
	}
	out := make([]ports.GraphEdge, 0)
	for _, e := range s.edges[orgID] {
		if !opts.IncludeDead && e.State == "TOMBSTONED" {
			continue
		}
		if len(wantRes) > 0 && !wantRes[e.FromID] && !wantRes[e.ToID] {
			continue
		}
		if len(wantPred) > 0 && !wantPred[e.Predicate] {
			continue
		}
		out = append(out, e)
		if len(out) >= opts.Limit {
			break
		}
	}
	return out, nil
}

func (s *Store) TombstoneEdge(_ context.Context, _ ports.Tx, orgID, edgeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.edges[orgID]
	if m == nil {
		return platform.ErrNotFound("edge not found")
	}
	e, ok := m[edgeID]
	if !ok {
		return platform.ErrNotFound("edge not found")
	}
	e.State = "TOMBSTONED"
	e.UpdatedAt = time.Now().UTC()
	m[edgeID] = e
	return nil
}

func (s *Store) GetRecordTx(_ context.Context, _ ports.Tx, orgID, resourceID string) (ports.Record, error) {
	return s.GetRecord(context.Background(), orgID, resourceID)
}

func (s *Store) InsertPlaceholder(_ context.Context, _ ports.Tx, rec ports.Record) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec.OrgID == "" || rec.ResourceID == "" {
		return false, platform.ErrValidation("org and resource required")
	}
	m := s.records[rec.OrgID]
	if m == nil {
		m = make(map[string]ports.Record)
		s.records[rec.OrgID] = m
	}
	if _, exists := m[rec.ResourceID]; exists {
		return false, nil
	}
	now := time.Now().UTC()
	rec.State = ports.LifecyclePlaceholder
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now
	if rec.Attributes == nil {
		rec.Attributes = map[string]string{}
	}
	rec.Attributes["placeholder"] = "true"
	m[rec.ResourceID] = rec
	return true, nil
}

func (s *Store) PromotePlaceholder(_ context.Context, _ ports.Tx, rec ports.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.records[rec.OrgID]
	if m == nil {
		m = make(map[string]ports.Record)
		s.records[rec.OrgID] = m
	}
	existing, ok := m[rec.ResourceID]
	now := time.Now().UTC()
	if ok && existing.State != ports.LifecyclePlaceholder && existing.State != ports.LifecycleEnsured {
		// Never clobber an authoritative accepted record via promote.
		rec.CreatedAt = existing.CreatedAt
	} else if ok {
		rec.CreatedAt = existing.CreatedAt
	} else if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	if rec.State == "" || rec.State == ports.LifecyclePlaceholder || rec.State == ports.LifecycleEnsured {
		rec.State = ports.LifecycleAccepted
	}
	rec.UpdatedAt = now
	if rec.Attributes == nil {
		rec.Attributes = map[string]string{}
	}
	delete(rec.Attributes, "placeholder")
	delete(rec.Attributes, "ensured")
	m[rec.ResourceID] = rec
	return nil
}

func (s *Store) EnqueueAuthzTuple(_ context.Context, _ ports.Tx, op ports.AuthzTupleOp) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if op.OrgID == "" || op.Object == "" || op.Relation == "" || op.Subject == "" {
		return platform.ErrValidation("authz tuple fields required")
	}
	if op.ID == "" {
		op.ID = platform.NewEventID()
	}
	if op.Operation == "" {
		op.Operation = "write"
	}
	if op.Status == "" {
		op.Status = "pending"
	}
	now := time.Now().UTC()
	if op.CreatedAt.IsZero() {
		op.CreatedAt = now
	}
	op.UpdatedAt = now
	if op.NextAttempt.IsZero() {
		op.NextAttempt = now
	}
	m := s.authzTuples[op.OrgID]
	if m == nil {
		m = make(map[string]ports.AuthzTupleOp)
		s.authzTuples[op.OrgID] = m
	}
	for id, existing := range m {
		if existing.Status == "pending" &&
			existing.Operation == op.Operation &&
			existing.Object == op.Object &&
			existing.Relation == op.Relation &&
			existing.Subject == op.Subject {
			existing.EdgeID = op.EdgeID
			existing.UpdatedAt = now
			m[id] = existing
			return nil
		}
	}
	m[op.ID] = op
	return nil
}

func (s *Store) ClaimAuthzTuples(_ context.Context, limit int, lease time.Duration) ([]ports.AuthzTupleOp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()
	until := now.Add(lease)
	out := make([]ports.AuthzTupleOp, 0, limit)
	for orgID, m := range s.authzTuples {
		for id, op := range m {
			if op.Status != "pending" {
				continue
			}
			if op.LeaseUntil != nil && op.LeaseUntil.After(now) {
				continue
			}
			if op.NextAttempt.After(now) {
				continue
			}
			op.LeaseUntil = &until
			op.UpdatedAt = now
			m[id] = op
			s.authzTuples[orgID] = m
			out = append(out, op)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func (s *Store) MarkAuthzTupleApplied(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for orgID, m := range s.authzTuples {
		if op, ok := m[id]; ok {
			op.Status = "applied"
			op.AppliedAt = &at
			op.UpdatedAt = at
			op.LeaseUntil = nil
			op.LastError = ""
			m[id] = op
			s.authzTuples[orgID] = m
			return nil
		}
	}
	return platform.ErrNotFound("authz tuple not found")
}

func (s *Store) MarkAuthzTupleFailed(_ context.Context, id string, attempts int, next time.Time, errMsg string, dead bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for orgID, m := range s.authzTuples {
		if op, ok := m[id]; ok {
			op.Attempts = attempts
			op.NextAttempt = next
			op.LastError = errMsg
			op.LeaseUntil = nil
			op.UpdatedAt = time.Now().UTC()
			if dead {
				op.Status = "dead"
			}
			m[id] = op
			s.authzTuples[orgID] = m
			return nil
		}
	}
	return platform.ErrNotFound("authz tuple not found")
}

func (s *Store) CountAuthzTuplePending(_ context.Context, orgID string) (int, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pending, dead := 0, 0
	for oid, m := range s.authzTuples {
		if orgID != "" && oid != orgID {
			continue
		}
		for _, op := range m {
			switch op.Status {
			case "pending":
				pending++
			case "dead":
				dead++
			}
		}
	}
	return pending, dead, nil
}

func (s *Store) ListActiveParentEdgesNeedingAuthz(_ context.Context, orgID string, limit int) ([]ports.GraphEdge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	out := make([]ports.GraphEdge, 0)
	for oid, m := range s.edges {
		if orgID != "" && oid != orgID {
			continue
		}
		for _, e := range m {
			if e.State == "ACTIVE" && e.Predicate == ports.EdgeParent && e.SyncAuthz {
				out = append(out, e)
				if len(out) >= limit {
					return out, nil
				}
			}
		}
	}
	return out, nil
}

func (s *Store) HasAuthzTupleCoverage(_ context.Context, orgID, operation, object, relation, subject string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if operation == "" {
		operation = "write"
	}
	for oid, m := range s.authzTuples {
		if orgID != "" && oid != orgID {
			continue
		}
		for _, op := range m {
			if op.Operation != operation {
				continue
			}
			if op.Object == object && op.Relation == relation && op.Subject == subject {
				if op.Status == "pending" || op.Status == "applied" {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// AppendAudit implements audit.LedgerWriter.
func (s *Store) AppendAudit(_ context.Context, event ports.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audits[event.OrgID] = append(s.audits[event.OrgID], event)
	return nil
}

// ListAudit returns org-scoped audit events.
func (s *Store) ListAudit(_ context.Context, orgID string, limit int) ([]ports.AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ev := s.audits[orgID]
	if limit <= 0 || limit > len(ev) {
		limit = len(ev)
	}
	out := make([]ports.AuditEvent, limit)
	copy(out, ev[len(ev)-limit:])
	return out, nil
}

// AppendChange stores a change feed item.
func (s *Store) AppendChange(_ context.Context, ev ports.ChangeEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.changes[ev.OrgID] = append(s.changes[ev.OrgID], ev)
	return nil
}

// ListChanges returns change events after cursor.
func (s *Store) ListChanges(_ context.Context, orgID, cursor string, limit int) ([]ports.ChangeEvent, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	all := s.changes[orgID]
	out := make([]ports.ChangeEvent, 0, limit)
	next := cursor
	for _, ev := range all {
		if cursor != "" && ev.Cursor <= cursor {
			continue
		}
		out = append(out, ev)
		next = ev.Cursor
		if len(out) >= limit {
			break
		}
	}
	return out, next, nil
}

func (s *Store) GetQuotas(_ context.Context, orgID string) (ports.Quota, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q, ok := s.quotas[orgID]
	if !ok {
		return ports.Quota{SearchPerMinute: 60, IntakePerMinute: 120, ExportPerMinute: 10, MaxResults: 25}, nil
	}
	return q, nil
}

func (s *Store) SetQuotas(_ context.Context, orgID string, q ports.Quota) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quotas[orgID] = q
	return nil
}
