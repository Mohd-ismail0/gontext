package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/xsama/context-fabric/internal/mapping"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// Pool wraps pgxpool with tenant-scoped helpers.
type Pool struct {
	*pgxpool.Pool
}

// Connect opens a pgx pool from DSN.
func Connect(ctx context.Context, dsn string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := p.Ping(ctx); err != nil {
		p.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Pool{Pool: p}, nil
}

// Store implements ports.LedgerStore against PostgreSQL/RLS.
type Store struct {
	pool *Pool
}

// NewStore wraps a pool as LedgerStore.
func NewStore(pool *Pool) *Store {
	return &Store{pool: pool}
}

var _ ports.LedgerStore = (*Store)(nil)

type pgTx struct {
	tx pgx.Tx
}

func (t *pgTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *pgTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

// WithTenant begins a transaction, sets app.organization_id (local), runs fn, commits.
func (p *Pool) WithTenant(ctx context.Context, orgID string, fn func(ctx context.Context, tx pgx.Tx) error) error {
	if orgID == "" {
		return platform.ErrValidation("organization_id required")
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, orgID); err != nil {
		return fmt.Errorf("set tenant: %w", err)
	}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// WithOrgTx implements ports.LedgerStore.
func (s *Store) WithOrgTx(ctx context.Context, orgID string, fn func(ctx context.Context, tx ports.Tx) error) error {
	return s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		return fn(ctx, &pgTx{tx: tx})
	})
}

func (s *Store) CreateOrganization(ctx context.Context, org ports.Organization) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO organizations (id, name, attributes, created_at) VALUES ($1,$2,$3,$4)`,
		org.ID, org.Name, mustJSON(org.Attributes), org.CreatedAt)
	return err
}

func (s *Store) GetOrganization(ctx context.Context, orgID string) (ports.Organization, error) {
	var o ports.Organization
	var attrs []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, attributes, created_at FROM organizations WHERE id=$1`, orgID).
		Scan(&o.ID, &o.Name, &attrs, &o.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return o, platform.ErrNotFound("organization not found")
		}
		return o, err
	}
	_ = json.Unmarshal(attrs, &o.Attributes)
	return o, nil
}

func (s *Store) UpsertRecord(ctx context.Context, tx ports.Tx, rec ports.Record) error {
	pg := mustPgTx(tx)
	_, err := pg.Exec(ctx, `
INSERT INTO resources (id, organization_id, resource_type, title, classification, tags, source_system, source_external_id, current_revision_id, lifecycle_state, attributes, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (organization_id, id) DO UPDATE SET
  title=EXCLUDED.title, classification=EXCLUDED.classification, tags=EXCLUDED.tags,
  current_revision_id=EXCLUDED.current_revision_id, lifecycle_state=EXCLUDED.lifecycle_state,
  attributes=EXCLUDED.attributes, updated_at=EXCLUDED.updated_at`,
		rec.ResourceID, rec.OrgID, nullStr(rec.Kind, "document"), rec.Title, nullStr(rec.Classification, "internal"),
		rec.Labels, rec.SourceSystem, rec.ExternalID, rec.CurrentRevID, nullStr(rec.State, "ACCEPTED"),
		mustJSON(rec.Attributes), rec.CreatedAt, rec.UpdatedAt)
	return err
}

func (s *Store) GetRecord(ctx context.Context, orgID, resourceID string) (ports.Record, error) {
	var r ports.Record
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var attrs []byte
		err := tx.QueryRow(ctx, `
SELECT id, organization_id, resource_type, COALESCE(title,''), COALESCE(classification,'internal'),
       COALESCE(tags,'{}'), COALESCE(source_system,''), COALESCE(source_external_id,''),
       COALESCE(current_revision_id,''), COALESCE(lifecycle_state,''), attributes, created_at, updated_at
FROM resources WHERE organization_id=$1 AND id=$2`, orgID, resourceID).
			Scan(&r.ResourceID, &r.OrgID, &r.Kind, &r.Title, &r.Classification, &r.Labels,
				&r.SourceSystem, &r.ExternalID, &r.CurrentRevID, &r.State, &attrs, &r.CreatedAt, &r.UpdatedAt)
		if err == pgx.ErrNoRows {
			return platform.ErrNotFound("record not found")
		}
		if err != nil {
			return err
		}
		_ = json.Unmarshal(attrs, &r.Attributes)
		return nil
	})
	return r, err
}

func (s *Store) ListRecords(ctx context.Context, orgID string, limit int, _ string) ([]ports.Record, string, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []ports.Record
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT id, organization_id, resource_type, COALESCE(title,''), COALESCE(classification,'internal'),
       COALESCE(current_revision_id,''), created_at, updated_at
FROM resources WHERE organization_id=$1 LIMIT $2`, orgID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r ports.Record
			if err := rows.Scan(&r.ResourceID, &r.OrgID, &r.Kind, &r.Title, &r.Classification, &r.CurrentRevID, &r.CreatedAt, &r.UpdatedAt); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, "", err
}

func (s *Store) AppendRevision(ctx context.Context, tx ports.Tx, rev ports.Revision) error {
	pg := mustPgTx(tx)
	_, err := pg.Exec(ctx, `
INSERT INTO revisions (id, organization_id, resource_id, sequence, content_hash, evidence_ref, state, attributes, observed_at, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		rev.RevisionID, rev.OrgID, rev.ResourceID, rev.Sequence, rev.ContentHash, rev.EvidenceRef,
		rev.State, mustJSON(rev.Attributes), rev.ObservedAt, rev.CreatedAt)
	return err
}

func (s *Store) GetRevision(ctx context.Context, orgID, revisionID string) (ports.Revision, error) {
	var r ports.Revision
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var attrs []byte
		err := tx.QueryRow(ctx, `
SELECT id, organization_id, resource_id, sequence, COALESCE(content_hash,''), COALESCE(evidence_ref,''),
       state, attributes, observed_at, created_at
FROM revisions WHERE organization_id=$1 AND id=$2`, orgID, revisionID).
			Scan(&r.RevisionID, &r.OrgID, &r.ResourceID, &r.Sequence, &r.ContentHash, &r.EvidenceRef,
				&r.State, &attrs, &r.ObservedAt, &r.CreatedAt)
		if err == pgx.ErrNoRows {
			return platform.ErrNotFound("revision not found")
		}
		if err != nil {
			return err
		}
		_ = json.Unmarshal(attrs, &r.Attributes)
		return nil
	})
	return r, err
}

func (s *Store) ListRevisions(ctx context.Context, orgID, resourceID string, limit int) ([]ports.Revision, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []ports.Revision
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT id, organization_id, resource_id, sequence, state, observed_at, created_at
FROM revisions WHERE organization_id=$1 AND resource_id=$2 ORDER BY sequence DESC LIMIT $3`,
			orgID, resourceID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r ports.Revision
			if err := rows.Scan(&r.RevisionID, &r.OrgID, &r.ResourceID, &r.Sequence, &r.State, &r.ObservedAt, &r.CreatedAt); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) EnqueueOutbox(ctx context.Context, tx ports.Tx, entry ports.OutboxEntry) error {
	pg := mustPgTx(tx)
	_, err := pg.Exec(ctx, `
INSERT INTO outbox (id, organization_id, subject, payload, headers, created_at)
VALUES ($1,$2,$3,$4,$5,$6)`,
		entry.ID, entry.OrgID, entry.Subject, entry.Payload, mustJSON(entry.Headers), entry.CreatedAt)
	return err
}

func (s *Store) ClaimOutbox(ctx context.Context, limit int) ([]ports.OutboxEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, organization_id, subject, payload, headers, created_at, published_at
FROM outbox WHERE published_at IS NULL ORDER BY created_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ports.OutboxEntry
	for rows.Next() {
		var e ports.OutboxEntry
		var headers []byte
		if err := rows.Scan(&e.ID, &e.OrgID, &e.Subject, &e.Payload, &headers, &e.CreatedAt, &e.Published); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(headers, &e.Headers)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListOutboxPending returns unpublished outbox rows without leasing them.
func (s *Store) ListOutboxPending(ctx context.Context, orgID string, limit int) ([]ports.OutboxEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `
SELECT id, organization_id, subject, payload, headers, created_at, published_at
FROM outbox WHERE published_at IS NULL`
	args := []any{}
	if orgID != "" {
		q += ` AND organization_id=$1`
		args = append(args, orgID)
		q += ` ORDER BY created_at ASC LIMIT $2`
		args = append(args, limit)
	} else {
		q += ` ORDER BY created_at ASC LIMIT $1`
		args = append(args, limit)
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ports.OutboxEntry
	for rows.Next() {
		var e ports.OutboxEntry
		var headers []byte
		if err := rows.Scan(&e.ID, &e.OrgID, &e.Subject, &e.Payload, &headers, &e.CreatedAt, &e.Published); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(headers, &e.Headers)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) MarkOutboxPublished(ctx context.Context, ids []string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE outbox SET published_at=$1 WHERE id = ANY($2)`, at, ids)
	return err
}

func (s *Store) PutInbox(ctx context.Context, entry ports.InboxEntry) error {
	return s.pool.WithTenant(ctx, entry.OrgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
INSERT INTO consumer_receipts (id, organization_id, consumer, msg_id, processed_at)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (organization_id, consumer, msg_id) DO NOTHING`,
			entry.ID, entry.OrgID, entry.Consumer, entry.MsgID, entry.ProcessedAt)
		return err
	})
}

func (s *Store) HasInbox(ctx context.Context, orgID, consumer, msgID string) (bool, error) {
	var exists bool
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM consumer_receipts WHERE organization_id=$1 AND consumer=$2 AND msg_id=$3)`,
			orgID, consumer, msgID).Scan(&exists)
	})
	return exists, err
}

func (s *Store) GetIdempotency(ctx context.Context, orgID, key string) (ports.IdempotencyRecord, error) {
	var rec ports.IdempotencyRecord
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
SELECT organization_id, idempotency_key, COALESCE(attributes->>'event_id',''), resource_id, revision_id, system_time
FROM records WHERE organization_id=$1 AND idempotency_key=$2
LIMIT 1`, orgID, key).
			Scan(&rec.OrgID, &rec.IdempotencyKey, &rec.EventID, &rec.ResourceID, &rec.RevisionID, &rec.CreatedAt)
		if err == pgx.ErrNoRows {
			return platform.ErrNotFound("idempotency key not found")
		}
		return err
	})
	return rec, err
}

func (s *Store) PutIdempotency(ctx context.Context, tx ports.Tx, rec ports.IdempotencyRecord) error {
	pg := mustPgTx(tx)
	attrs := mustJSON(map[string]string{"event_id": rec.EventID})
	_, err := pg.Exec(ctx, `
INSERT INTO records (organization_id, resource_id, revision_id, classification, visibility_ref, lifecycle_state, idempotency_key, attributes, system_time)
VALUES ($1,$2,$3,'internal','','ACCEPTED',$4,$5,$6)
ON CONFLICT (organization_id, idempotency_key) DO NOTHING`,
		rec.OrgID, rec.ResourceID, rec.RevisionID, rec.IdempotencyKey, attrs, rec.CreatedAt)
	if err != nil {
		return err
	}
	// Detect conflict: if row missing for this key with our revision, treat as conflict.
	var exists bool
	if err := pg.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM records WHERE organization_id=$1 AND idempotency_key=$2 AND revision_id=$3)`,
		rec.OrgID, rec.IdempotencyKey, rec.RevisionID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return platform.ErrConflict("idempotency key exists")
	}
	return nil
}

func (s *Store) UpsertSource(ctx context.Context, tx ports.Tx, src ports.SourceRegistration) error {
	pg := mustPgTx(tx)
	_, err := pg.Exec(ctx, `
INSERT INTO sources (id, organization_id, system, display_name, trust_tier, mapping_spec_id, enabled, attributes, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (organization_id, id) DO UPDATE SET
  display_name=EXCLUDED.display_name, trust_tier=EXCLUDED.trust_tier, enabled=EXCLUDED.enabled,
  attributes=EXCLUDED.attributes, updated_at=EXCLUDED.updated_at`,
		src.SourceID, src.OrgID, src.System, src.DisplayName, src.TrustTier, src.MappingSpec,
		src.Enabled, mustJSON(src.Attributes), src.CreatedAt, src.UpdatedAt)
	return err
}

func (s *Store) GetSource(ctx context.Context, orgID, sourceID string) (ports.SourceRegistration, error) {
	var src ports.SourceRegistration
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var attrs []byte
		err := tx.QueryRow(ctx, `
SELECT id, organization_id, system, COALESCE(display_name,''), trust_tier, COALESCE(mapping_spec_id,''),
       enabled, attributes, created_at, updated_at
FROM sources WHERE organization_id=$1 AND id=$2`, orgID, sourceID).
			Scan(&src.SourceID, &src.OrgID, &src.System, &src.DisplayName, &src.TrustTier, &src.MappingSpec,
				&src.Enabled, &attrs, &src.CreatedAt, &src.UpdatedAt)
		if err == pgx.ErrNoRows {
			return platform.ErrNotFound("source not found")
		}
		if err != nil {
			return err
		}
		_ = json.Unmarshal(attrs, &src.Attributes)
		return nil
	})
	return src, err
}

func (s *Store) ListSources(ctx context.Context, orgID string) ([]ports.SourceRegistration, error) {
	var out []ports.SourceRegistration
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT id, organization_id, system, COALESCE(display_name,''), trust_tier, enabled, created_at, updated_at
FROM sources WHERE organization_id=$1`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var src ports.SourceRegistration
			if err := rows.Scan(&src.SourceID, &src.OrgID, &src.System, &src.DisplayName, &src.TrustTier, &src.Enabled, &src.CreatedAt, &src.UpdatedAt); err != nil {
				return err
			}
			out = append(out, src)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) CreateDelegation(ctx context.Context, tx ports.Tx, grant ports.DelegationGrant) error {
	pg := mustPgTx(tx)
	_, err := pg.Exec(ctx, `
INSERT INTO delegation_grants (id, organization_id, subject_id, actor_id, owner_id, actions, resource_ids, purposes, expires_at, revoked, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		grant.ID, grant.OrgID, grant.SubjectID, grant.ActorID, grant.OwnerID, grant.Actions,
		grant.ResourceIDs, grant.Purposes, grant.ExpiresAt, grant.Revoked, grant.CreatedAt)
	return err
}

func (s *Store) GetDelegation(ctx context.Context, orgID, grantID string) (ports.DelegationGrant, error) {
	var g ports.DelegationGrant
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
SELECT id, organization_id, subject_id, actor_id, COALESCE(owner_id,''), actions, resource_ids, purposes, expires_at, revoked, created_at
FROM delegation_grants WHERE organization_id=$1 AND id=$2`, orgID, grantID).
			Scan(&g.ID, &g.OrgID, &g.SubjectID, &g.ActorID, &g.OwnerID, &g.Actions, &g.ResourceIDs, &g.Purposes, &g.ExpiresAt, &g.Revoked, &g.CreatedAt)
		if err == pgx.ErrNoRows {
			return platform.ErrNotFound("delegation not found")
		}
		return err
	})
	return g, err
}

func (s *Store) RevokeDelegation(ctx context.Context, tx ports.Tx, orgID, grantID string) error {
	pg := mustPgTx(tx)
	ct, err := pg.Exec(ctx, `UPDATE delegation_grants SET revoked=true WHERE organization_id=$1 AND id=$2`, orgID, grantID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return platform.ErrNotFound("delegation not found")
	}
	return nil
}

func (s *Store) ListDelegations(ctx context.Context, orgID, actorID string) ([]ports.DelegationGrant, error) {
	var out []ports.DelegationGrant
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		q := `SELECT id, organization_id, subject_id, actor_id, COALESCE(owner_id,''), actions, resource_ids, purposes, expires_at, revoked, created_at
FROM delegation_grants WHERE organization_id=$1`
		args := []any{orgID}
		if actorID != "" {
			q += ` AND actor_id=$2`
			args = append(args, actorID)
		}
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var g ports.DelegationGrant
			if err := rows.Scan(&g.ID, &g.OrgID, &g.SubjectID, &g.ActorID, &g.OwnerID, &g.Actions, &g.ResourceIDs, &g.Purposes, &g.ExpiresAt, &g.Revoked, &g.CreatedAt); err != nil {
				return err
			}
			out = append(out, g)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) UpsertEdge(ctx context.Context, tx ports.Tx, edge ports.GraphEdge) error {
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
	pg := mustPgTx(tx)
	// Idempotent update of existing active triple.
	var existingID string
	err := pg.QueryRow(ctx, `
SELECT id FROM graph_edges
WHERE organization_id=$1 AND from_id=$2 AND to_id=$3 AND predicate=$4 AND lifecycle_state='ACTIVE'
LIMIT 1`, edge.OrgID, edge.FromID, edge.ToID, edge.Predicate).Scan(&existingID)
	if err == nil && existingID != "" {
		_, err = pg.Exec(ctx, `
UPDATE graph_edges SET confidence=$1, sync_authz=$2, attributes=$3, updated_at=$4
WHERE organization_id=$5 AND id=$6`,
			edge.Confidence, edge.SyncAuthz, mustJSON(edge.Attributes), edge.UpdatedAt, edge.OrgID, existingID)
		return err
	}
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	_, err = pg.Exec(ctx, `
INSERT INTO graph_edges (id, organization_id, from_id, to_id, predicate, confidence, lifecycle_state, sync_authz, attributes, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (organization_id, id) DO UPDATE SET
  from_id=EXCLUDED.from_id, to_id=EXCLUDED.to_id, predicate=EXCLUDED.predicate,
  confidence=EXCLUDED.confidence, lifecycle_state=EXCLUDED.lifecycle_state,
  sync_authz=EXCLUDED.sync_authz, attributes=EXCLUDED.attributes, updated_at=EXCLUDED.updated_at`,
		edge.EdgeID, edge.OrgID, edge.FromID, edge.ToID, edge.Predicate, edge.Confidence,
		edge.State, edge.SyncAuthz, mustJSON(edge.Attributes), edge.CreatedAt, edge.UpdatedAt)
	return err
}

func (s *Store) GetEdge(ctx context.Context, orgID, edgeID string) (ports.GraphEdge, error) {
	var e ports.GraphEdge
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var attrs []byte
		err := tx.QueryRow(ctx, `
SELECT id, organization_id, from_id, to_id, predicate, confidence, lifecycle_state, COALESCE(sync_authz,false), attributes, created_at, updated_at
FROM graph_edges WHERE organization_id=$1 AND id=$2`, orgID, edgeID).
			Scan(&e.EdgeID, &e.OrgID, &e.FromID, &e.ToID, &e.Predicate, &e.Confidence, &e.State, &e.SyncAuthz, &attrs, &e.CreatedAt, &e.UpdatedAt)
		if err == pgx.ErrNoRows {
			return platform.ErrNotFound("edge not found")
		}
		if err != nil {
			return err
		}
		_ = json.Unmarshal(attrs, &e.Attributes)
		return nil
	})
	return e, err
}

func (s *Store) ListEdges(ctx context.Context, orgID string, opts ports.EdgeListOptions) ([]ports.GraphEdge, error) {
	if opts.Limit <= 0 {
		opts.Limit = 200
	}
	var out []ports.GraphEdge
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		q := `
SELECT id, organization_id, from_id, to_id, predicate, confidence, lifecycle_state, COALESCE(sync_authz,false), attributes, created_at, updated_at
FROM graph_edges WHERE organization_id=$1`
		args := []any{orgID}
		argN := 2
		if !opts.IncludeDead {
			q += ` AND lifecycle_state='ACTIVE'`
		}
		if len(opts.ResourceIDs) > 0 {
			q += fmt.Sprintf(` AND (from_id = ANY($%d) OR to_id = ANY($%d))`, argN, argN)
			args = append(args, opts.ResourceIDs)
			argN++
		}
		if len(opts.Predicates) > 0 {
			q += fmt.Sprintf(` AND predicate = ANY($%d)`, argN)
			args = append(args, opts.Predicates)
			argN++
		}
		q += fmt.Sprintf(` ORDER BY from_id ASC, predicate ASC, to_id ASC, created_at ASC, id ASC LIMIT $%d`, argN)
		args = append(args, opts.Limit)

		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e ports.GraphEdge
			var attrs []byte
			if err := rows.Scan(&e.EdgeID, &e.OrgID, &e.FromID, &e.ToID, &e.Predicate, &e.Confidence, &e.State, &e.SyncAuthz, &attrs, &e.CreatedAt, &e.UpdatedAt); err != nil {
				return err
			}
			_ = json.Unmarshal(attrs, &e.Attributes)
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) TombstoneEdge(ctx context.Context, tx ports.Tx, orgID, edgeID string) error {
	pg := mustPgTx(tx)
	ct, err := pg.Exec(ctx, `
UPDATE graph_edges SET lifecycle_state='TOMBSTONED', updated_at=now()
WHERE organization_id=$1 AND id=$2`, orgID, edgeID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return platform.ErrNotFound("edge not found")
	}
	return nil
}

func (s *Store) GetRecordTx(ctx context.Context, tx ports.Tx, orgID, resourceID string) (ports.Record, error) {
	pg := mustPgTx(tx)
	var r ports.Record
	var attrs []byte
	err := pg.QueryRow(ctx, `
SELECT id, organization_id, resource_type, COALESCE(title,''), COALESCE(classification,'internal'),
       COALESCE(tags,'{}'), COALESCE(source_system,''), COALESCE(source_external_id,''),
       COALESCE(current_revision_id,''), COALESCE(lifecycle_state,''), attributes, created_at, updated_at
FROM resources WHERE organization_id=$1 AND id=$2`, orgID, resourceID).
		Scan(&r.ResourceID, &r.OrgID, &r.Kind, &r.Title, &r.Classification, &r.Labels,
			&r.SourceSystem, &r.ExternalID, &r.CurrentRevID, &r.State, &attrs, &r.CreatedAt, &r.UpdatedAt)
	if err == pgx.ErrNoRows {
		return r, platform.ErrNotFound("record not found")
	}
	if err != nil {
		return r, err
	}
	_ = json.Unmarshal(attrs, &r.Attributes)
	return r, nil
}

func (s *Store) InsertPlaceholder(ctx context.Context, tx ports.Tx, rec ports.Record) (bool, error) {
	pg := mustPgTx(tx)
	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now
	if rec.State == "" {
		rec.State = ports.LifecyclePlaceholder
	}
	if rec.Attributes == nil {
		rec.Attributes = map[string]string{}
	}
	rec.Attributes["placeholder"] = "true"
	ct, err := pg.Exec(ctx, `
INSERT INTO resources (id, organization_id, resource_type, title, classification, tags, source_system, source_external_id, current_revision_id, lifecycle_state, attributes, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (organization_id, id) DO NOTHING`,
		rec.ResourceID, rec.OrgID, nullStr(rec.Kind, "resource"), rec.Title, nullStr(rec.Classification, "internal"),
		rec.Labels, rec.SourceSystem, rec.ExternalID, rec.CurrentRevID, rec.State,
		mustJSON(rec.Attributes), rec.CreatedAt, rec.UpdatedAt)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

func (s *Store) PromotePlaceholder(ctx context.Context, tx ports.Tx, rec ports.Record) error {
	pg := mustPgTx(tx)
	now := time.Now().UTC()
	rec.UpdatedAt = now
	if rec.State == "" || rec.State == ports.LifecyclePlaceholder || rec.State == ports.LifecycleEnsured {
		rec.State = ports.LifecycleAccepted
	}
	if rec.Attributes == nil {
		rec.Attributes = map[string]string{}
	}
	delete(rec.Attributes, "placeholder")
	delete(rec.Attributes, "ensured")
	_, err := pg.Exec(ctx, `
INSERT INTO resources (id, organization_id, resource_type, title, classification, tags, source_system, source_external_id, current_revision_id, lifecycle_state, attributes, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (organization_id, id) DO UPDATE SET
  title=EXCLUDED.title,
  classification=EXCLUDED.classification,
  tags=EXCLUDED.tags,
  source_system=EXCLUDED.source_system,
  source_external_id=EXCLUDED.source_external_id,
  current_revision_id=EXCLUDED.current_revision_id,
  lifecycle_state=EXCLUDED.lifecycle_state,
  attributes=EXCLUDED.attributes,
  updated_at=EXCLUDED.updated_at
WHERE resources.lifecycle_state IN ('PLACEHOLDER','ENSURED')
   OR resources.lifecycle_state = EXCLUDED.lifecycle_state
   OR TRUE`,
		rec.ResourceID, rec.OrgID, nullStr(rec.Kind, "document"), rec.Title, nullStr(rec.Classification, "internal"),
		rec.Labels, rec.SourceSystem, rec.ExternalID, rec.CurrentRevID, rec.State,
		mustJSON(rec.Attributes), coalesceTime(rec.CreatedAt, now), rec.UpdatedAt)
	return err
}

func coalesceTime(t, def time.Time) time.Time {
	if t.IsZero() {
		return def
	}
	return t
}

func (s *Store) EnqueueAuthzTuple(ctx context.Context, tx ports.Tx, op ports.AuthzTupleOp) error {
	pg := mustPgTx(tx)
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
	_, err := pg.Exec(ctx, `
INSERT INTO authz_tuple_outbox (id, organization_id, operation, object, relation, subject, edge_id, status, attempts, next_attempt, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,0,$9,$10,$11)
ON CONFLICT DO NOTHING`,
		op.ID, op.OrgID, op.Operation, op.Object, op.Relation, op.Subject, nullEmpty(op.EdgeID),
		op.Status, op.NextAttempt, op.CreatedAt, op.UpdatedAt)
	return err
}

func nullEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Store) ClaimAuthzTuples(ctx context.Context, limit int, lease time.Duration) ([]ports.AuthzTupleOp, error) {
	if limit <= 0 {
		limit = 20
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}
	var out []ports.AuthzTupleOp
	rows, err := s.pool.Query(ctx, `
UPDATE authz_tuple_outbox SET lease_until=$1, updated_at=now()
WHERE id IN (
  SELECT id FROM authz_tuple_outbox
  WHERE status='pending'
    AND next_attempt <= now()
    AND (lease_until IS NULL OR lease_until < now())
  ORDER BY next_attempt ASC
  LIMIT $2
  FOR UPDATE SKIP LOCKED
)
RETURNING id, organization_id, operation, object, relation, subject, COALESCE(edge_id,''), status, attempts,
          COALESCE(last_error,''), lease_until, next_attempt, created_at, updated_at, applied_at`,
		time.Now().UTC().Add(lease), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var op ports.AuthzTupleOp
		if err := rows.Scan(&op.ID, &op.OrgID, &op.Operation, &op.Object, &op.Relation, &op.Subject, &op.EdgeID,
			&op.Status, &op.Attempts, &op.LastError, &op.LeaseUntil, &op.NextAttempt, &op.CreatedAt, &op.UpdatedAt, &op.AppliedAt); err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return out, rows.Err()
}

func (s *Store) MarkAuthzTupleApplied(ctx context.Context, id string, at time.Time) error {
	ct, err := s.pool.Exec(ctx, `
UPDATE authz_tuple_outbox SET status='applied', applied_at=$1, lease_until=NULL, last_error='', updated_at=$1
WHERE id=$2`, at, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return platform.ErrNotFound("authz tuple not found")
	}
	return nil
}

func (s *Store) MarkAuthzTupleFailed(ctx context.Context, id string, attempts int, next time.Time, errMsg string, dead bool) error {
	status := "pending"
	if dead {
		status = "dead"
	}
	ct, err := s.pool.Exec(ctx, `
UPDATE authz_tuple_outbox SET status=$1, attempts=$2, next_attempt=$3, last_error=$4, lease_until=NULL, updated_at=now()
WHERE id=$5`, status, attempts, next, errMsg, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return platform.ErrNotFound("authz tuple not found")
	}
	return nil
}

func (s *Store) CountAuthzTuplePending(ctx context.Context, orgID string) (int, int, error) {
	var pending, dead int
	q := `SELECT
  COUNT(*) FILTER (WHERE status='pending'),
  COUNT(*) FILTER (WHERE status='dead')
FROM authz_tuple_outbox`
	args := []any{}
	if orgID != "" {
		q += ` WHERE organization_id=$1`
		args = append(args, orgID)
	}
	err := s.pool.QueryRow(ctx, q, args...).Scan(&pending, &dead)
	return pending, dead, err
}

func (s *Store) ListActiveParentEdgesNeedingAuthz(ctx context.Context, orgID string, limit int) ([]ports.GraphEdge, error) {
	if limit <= 0 {
		limit = 100
	}
	if orgID != "" {
		var out []ports.GraphEdge
		err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `
SELECT id, organization_id, from_id, to_id, predicate, confidence, lifecycle_state, COALESCE(sync_authz,false), attributes, created_at, updated_at
FROM graph_edges
WHERE organization_id=$1 AND lifecycle_state='ACTIVE' AND predicate='parent' AND sync_authz=true
ORDER BY created_at ASC LIMIT $2`, orgID, limit)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var e ports.GraphEdge
				var attrs []byte
				if err := rows.Scan(&e.EdgeID, &e.OrgID, &e.FromID, &e.ToID, &e.Predicate, &e.Confidence, &e.State, &e.SyncAuthz, &attrs, &e.CreatedAt, &e.UpdatedAt); err != nil {
					return err
				}
				_ = json.Unmarshal(attrs, &e.Attributes)
				out = append(out, e)
			}
			return rows.Err()
		})
		return out, err
	}
	// Cross-org reconcile path (worker): bypass tenant filter via SET LOCAL role if available.
	rows, err := s.pool.Query(ctx, `
SELECT id, organization_id, from_id, to_id, predicate, confidence, lifecycle_state, COALESCE(sync_authz,false), attributes, created_at, updated_at
FROM graph_edges
WHERE lifecycle_state='ACTIVE' AND predicate='parent' AND sync_authz=true
ORDER BY created_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ports.GraphEdge
	for rows.Next() {
		var e ports.GraphEdge
		var attrs []byte
		if err := rows.Scan(&e.EdgeID, &e.OrgID, &e.FromID, &e.ToID, &e.Predicate, &e.Confidence, &e.State, &e.SyncAuthz, &attrs, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(attrs, &e.Attributes)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) HasAuthzTupleCoverage(ctx context.Context, orgID, operation, object, relation, subject string) (bool, error) {
	if operation == "" {
		operation = "write"
	}
	var n int
	q := `SELECT COUNT(*) FROM authz_tuple_outbox
WHERE organization_id=$1 AND operation=$2 AND object=$3 AND relation=$4 AND subject=$5
  AND status IN ('pending','applied')`
	err := s.pool.QueryRow(ctx, q, orgID, operation, object, relation, subject).Scan(&n)
	return n > 0, err
}

// PutMappingSpec stores a MappingSpec in mapping_specs when available, else sources.attributes.
func (s *Store) PutMappingSpec(ctx context.Context, orgID, sourceID string, spec mapping.Spec) error {
	if orgID == "" || sourceID == "" {
		return platform.ErrValidation("org and source_id required")
	}
	if spec.OrganizationID == "" {
		spec.OrganizationID = orgID
	}
	if spec.SourceID == "" {
		spec.SourceID = sourceID
	}
	if spec.ID == "" {
		spec.ID = sourceID
	}
	version := spec.Revision
	if version == "" {
		version = "1"
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	return s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var hasTable bool
		if err := tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1 FROM information_schema.tables
  WHERE table_schema='public' AND table_name='mapping_specs'
)`).Scan(&hasTable); err != nil {
			return err
		}
		if hasTable {
			_, err := tx.Exec(ctx, `
INSERT INTO mapping_specs (id, organization_id, version, source_kind, rules, created_at)
VALUES ($1,$2,$3,$4,$5,now())
ON CONFLICT (organization_id, id, version) DO UPDATE SET rules=EXCLUDED.rules, source_kind=EXCLUDED.source_kind`,
				spec.ID, orgID, version, firstNonEmpty(spec.SourceID, sourceID), raw)
			if err == nil {
				_, _ = tx.Exec(ctx, `UPDATE sources SET mapping_spec_id=$1, updated_at=now() WHERE organization_id=$2 AND id=$3`,
					spec.ID, orgID, sourceID)
				return nil
			}
			// Fall through to attributes on schema mismatch / missing columns.
		}
		attrs := map[string]string{"mapping_spec": string(raw)}
		_, err := tx.Exec(ctx, `
UPDATE sources SET
  attributes = COALESCE(attributes, '{}'::jsonb) || $1::jsonb,
  mapping_spec_id = COALESCE(NULLIF(mapping_spec_id,''), $2),
  updated_at = now()
WHERE organization_id=$3 AND id=$4`,
			mustJSON(attrs), spec.ID, orgID, sourceID)
		return err
	})
}

// GetMappingSpec loads MappingSpec for a source.
func (s *Store) GetMappingSpec(ctx context.Context, orgID, sourceID string) (mapping.Spec, error) {
	var out mapping.Spec
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		var mappingSpecID string
		var attrs []byte
		err := tx.QueryRow(ctx, `
SELECT COALESCE(mapping_spec_id,''), attributes FROM sources WHERE organization_id=$1 AND id=$2`,
			orgID, sourceID).Scan(&mappingSpecID, &attrs)
		if err == pgx.ErrNoRows {
			return platform.ErrNotFound("mapping spec not found")
		}
		if err != nil {
			return err
		}
		var attrMap map[string]string
		_ = json.Unmarshal(attrs, &attrMap)
		if attrMap != nil {
			if raw, ok := attrMap["mapping_spec"]; ok && raw != "" {
				if err := json.Unmarshal([]byte(raw), &out); err == nil {
					return nil
				}
			}
		}
		id := mappingSpecID
		if id == "" {
			id = sourceID
		}
		var hasTable bool
		_ = tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1 FROM information_schema.tables
  WHERE table_schema='public' AND table_name='mapping_specs'
)`).Scan(&hasTable)
		if !hasTable {
			return platform.ErrNotFound("mapping spec not found")
		}
		var rules []byte
		err = tx.QueryRow(ctx, `
SELECT rules FROM mapping_specs
WHERE organization_id=$1 AND id=$2
ORDER BY created_at DESC LIMIT 1`, orgID, id).Scan(&rules)
		if err == pgx.ErrNoRows {
			return platform.ErrNotFound("mapping spec not found")
		}
		if err != nil {
			return err
		}
		if err := json.Unmarshal(rules, &out); err != nil {
			return platform.ErrValidation("invalid stored mapping spec")
		}
		return nil
	})
	return out, err
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// AppendAudit persists an audit event under tenant RLS.
func (s *Store) AppendAudit(ctx context.Context, event ports.AuditEvent) error {
	return s.pool.WithTenant(ctx, event.OrgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
INSERT INTO audit_events (id, organization_id, principal_id, principal_kind, delegation_id, action, reason_code,
  authz_model_rev, policy_revision, resource_count, resource_ids_sample, trace_id, attributes, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
			event.AuditID, event.OrgID, event.PrincipalID, string(event.PrincipalKind), event.DelegationID,
			event.Action, event.ReasonCode, event.AuthzModelRev, event.PolicyRevision, event.ResourceCount,
			event.ResourceIDsSample, event.TraceID, mustJSON(event.Attributes), event.CreatedAt)
		return err
	})
}

// ListAudit returns recent audit events for an organization (no-op empty if table missing).
func (s *Store) ListAudit(ctx context.Context, orgID string, limit int) ([]ports.AuditEvent, error) {
	ok, err := s.tableExists(ctx, "audit_events")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	var out []ports.AuditEvent
	err = s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT id, organization_id, principal_id, COALESCE(principal_kind,''), COALESCE(delegation_id,''),
       action, reason_code, COALESCE(authz_model_rev,''), COALESCE(policy_revision,''),
       resource_count, COALESCE(resource_ids_sample,'{}'), COALESCE(trace_id,''), attributes, created_at
FROM audit_events WHERE organization_id=$1 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ev ports.AuditEvent
			var kind string
			var attrs []byte
			if err := rows.Scan(&ev.AuditID, &ev.OrgID, &ev.PrincipalID, &kind, &ev.DelegationID,
				&ev.Action, &ev.ReasonCode, &ev.AuthzModelRev, &ev.PolicyRevision,
				&ev.ResourceCount, &ev.ResourceIDsSample, &ev.TraceID, &attrs, &ev.CreatedAt); err != nil {
				return err
			}
			ev.PrincipalKind = ports.PrincipalKind(kind)
			_ = json.Unmarshal(attrs, &ev.Attributes)
			out = append(out, ev)
		}
		return rows.Err()
	})
	return out, err
}

// AppendChange writes a change_events row.
func (s *Store) AppendChange(ctx context.Context, ev ports.ChangeEvent) error {
	if ev.EventID == "" {
		ev.EventID = platform.NewEventID()
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}
	if ev.Cursor == "" {
		ev.Cursor = ev.OccurredAt.UTC().Format(time.RFC3339Nano)
	}
	return s.pool.WithTenant(ctx, ev.OrgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
INSERT INTO change_events (id, organization_id, resource_id, revision_id, action, cursor, occurred_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			ev.EventID, ev.OrgID, ev.ResourceID, nullEmpty(ev.RevisionID), ev.Action, ev.Cursor, ev.OccurredAt)
		return err
	})
}

// ListChanges returns change events after cursor (lexicographic on cursor column).
func (s *Store) ListChanges(ctx context.Context, orgID, cursor string, limit int) ([]ports.ChangeEvent, string, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []ports.ChangeEvent
	next := cursor
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT id, organization_id, resource_id, COALESCE(revision_id,''), action, cursor, occurred_at
FROM change_events
WHERE organization_id=$1 AND ($2 = '' OR cursor > $2)
ORDER BY cursor ASC
LIMIT $3`, orgID, cursor, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ev ports.ChangeEvent
			if err := rows.Scan(&ev.EventID, &ev.OrgID, &ev.ResourceID, &ev.RevisionID, &ev.Action, &ev.Cursor, &ev.OccurredAt); err != nil {
				return err
			}
			out = append(out, ev)
			next = ev.Cursor
		}
		return rows.Err()
	})
	return out, next, err
}

// GetQuotas loads org quotas (defaults when no row).
func (s *Store) GetQuotas(ctx context.Context, orgID string) (ports.Quota, error) {
	def := ports.Quota{SearchPerMinute: 60, IntakePerMinute: 120, ExportPerMinute: 10, MaxResults: 25}
	var q ports.Quota
	err := s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
SELECT search_per_minute, intake_per_minute, export_per_minute, max_results
FROM quotas WHERE organization_id=$1`, orgID).
			Scan(&q.SearchPerMinute, &q.IntakePerMinute, &q.ExportPerMinute, &q.MaxResults)
		if err == pgx.ErrNoRows {
			q = def
			return nil
		}
		return err
	})
	return q, err
}

// SetQuotas upserts org quotas.
func (s *Store) SetQuotas(ctx context.Context, orgID string, q ports.Quota) error {
	return s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
INSERT INTO quotas (organization_id, search_per_minute, intake_per_minute, export_per_minute, max_results, updated_at)
VALUES ($1,$2,$3,$4,$5,now())
ON CONFLICT (organization_id) DO UPDATE SET
  search_per_minute=EXCLUDED.search_per_minute,
  intake_per_minute=EXCLUDED.intake_per_minute,
  export_per_minute=EXCLUDED.export_per_minute,
  max_results=EXCLUDED.max_results,
  updated_at=now()`,
			orgID, q.SearchPerMinute, q.IntakePerMinute, q.ExportPerMinute, q.MaxResults)
		return err
	})
}

// PutExportJob upserts an export_jobs row (manifest JSON stored in manifest_uri).
func (s *Store) PutExportJob(ctx context.Context, orgID, jobID, status string, manifest any) error {
	ok, err := s.tableExists(ctx, "export_jobs")
	if err != nil {
		return err
	}
	if !ok {
		return platform.ErrUnavailable("export_jobs table missing; run migrate")
	}
	manifestJSON := string(mustJSON(manifest))
	return s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
INSERT INTO export_jobs (id, organization_id, status, manifest_uri, created_at)
VALUES ($1,$2,$3,$4,now())
ON CONFLICT (organization_id, id) DO UPDATE SET
  status=EXCLUDED.status,
  manifest_uri=EXCLUDED.manifest_uri,
  completed_at=CASE WHEN EXCLUDED.status IN ('completed','failed') THEN now() ELSE export_jobs.completed_at END`,
			jobID, orgID, nullStr(status, "pending"), manifestJSON)
		return err
	})
}

// GetExportJob loads an export job; manifest is unmarshaled from manifest_uri JSON when possible.
func (s *Store) GetExportJob(ctx context.Context, orgID, jobID string) (string, string, any, error) {
	ok, err := s.tableExists(ctx, "export_jobs")
	if err != nil {
		return "", "", nil, err
	}
	if !ok {
		return "", "", nil, platform.ErrNotFound("export not found")
	}
	var id, status, manifestURI string
	err = s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
SELECT id, status, COALESCE(manifest_uri,'')
FROM export_jobs WHERE organization_id=$1 AND id=$2`, orgID, jobID).
			Scan(&id, &status, &manifestURI)
		if err == pgx.ErrNoRows {
			return platform.ErrNotFound("export not found")
		}
		return err
	})
	if err != nil {
		return "", "", nil, err
	}
	var manifest any
	if manifestURI != "" {
		var decoded any
		if json.Unmarshal([]byte(manifestURI), &decoded) == nil {
			manifest = decoded
		} else {
			manifest = manifestURI
		}
	}
	return id, status, manifest, nil
}

// PutAccessRequest persists a pending access request (access_requests table).
func (s *Store) PutAccessRequest(ctx context.Context, orgID, requestID, resourceID, purpose, justification, auditID string) error {
	if orgID == "" || requestID == "" {
		return platform.ErrValidation("org and request_id required")
	}
	ok, err := s.tableExists(ctx, "access_requests")
	if err != nil {
		return err
	}
	if !ok {
		return platform.ErrUnavailable("access_requests table missing; run migrate")
	}
	return s.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
INSERT INTO access_requests (id, organization_id, resource_id, purpose, justification, status, audit_id, created_at)
VALUES ($1,$2,$3,$4,$5,'pending',$6,now())
ON CONFLICT (organization_id, id) DO UPDATE SET
  resource_id=EXCLUDED.resource_id,
  purpose=EXCLUDED.purpose,
  justification=EXCLUDED.justification,
  audit_id=EXCLUDED.audit_id`,
			requestID, orgID, resourceID, purpose, justification, nullEmpty(auditID))
		return err
	})
}

func (s *Store) tableExists(ctx context.Context, name string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1 FROM information_schema.tables
  WHERE table_schema='public' AND table_name=$1
)`, name).Scan(&ok)
	return ok, err
}

func mustPgTx(tx ports.Tx) pgx.Tx {
	if t, ok := tx.(*pgTx); ok {
		return t.tx
	}
	panic("postgres: unexpected tx type")
}

func mustJSON(v any) []byte {
	if v == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func nullStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
