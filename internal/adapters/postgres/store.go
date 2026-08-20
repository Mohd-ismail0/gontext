package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
