-- Context Fabric canonical ledger schema (v1)
-- Tenant tables require organization_id NOT NULL and FORCE ROW LEVEL SECURITY.
-- Application role context_gateway must remain NOBYPASSRLS (see deploy/postgres/init).

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Optional: prefer pgvector when available. BYTEA stores serialized vectors otherwise.
-- CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS organizations (
  id            TEXT PRIMARY KEY,
  name          TEXT NOT NULL,
  attributes    JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS context_spaces (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL REFERENCES organizations(id),
  name             TEXT NOT NULL,
  attributes       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE TABLE IF NOT EXISTS principals (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL REFERENCES organizations(id),
  kind             TEXT NOT NULL,
  subject          TEXT NOT NULL,
  issuer           TEXT,
  roles            TEXT[] NOT NULL DEFAULT '{}',
  attributes       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE TABLE IF NOT EXISTS resources (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL REFERENCES organizations(id),
  context_space_id TEXT,
  brand_id         TEXT,
  resource_type    TEXT NOT NULL,
  title            TEXT,
  classification   TEXT NOT NULL DEFAULT 'internal',
  visibility_ref   TEXT NOT NULL DEFAULT '',
  purpose_allowlist TEXT[] NOT NULL DEFAULT '{}',
  tags             TEXT[] NOT NULL DEFAULT '{}',
  source_system    TEXT,
  source_external_id TEXT,
  source_revision  TEXT,
  current_revision_id TEXT,
  lifecycle_state  TEXT NOT NULL DEFAULT 'RECEIVED',
  attributes       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id),
  UNIQUE (organization_id, source_system, source_external_id, source_revision)
);

CREATE INDEX IF NOT EXISTS idx_resources_org_resource
  ON resources (organization_id, id);

CREATE TABLE IF NOT EXISTS revisions (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  resource_id      TEXT NOT NULL,
  sequence         BIGINT NOT NULL DEFAULT 0,
  content_hash     TEXT,
  evidence_ref     TEXT,
  state            TEXT NOT NULL,
  attributes       JSONB NOT NULL DEFAULT '{}'::jsonb,
  observed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id),
  FOREIGN KEY (organization_id, resource_id) REFERENCES resources (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_revisions_org_resource
  ON revisions (organization_id, resource_id);

-- Canonical record metadata projection (retrieval-safe fields only).
CREATE TABLE IF NOT EXISTS records (
  organization_id  TEXT NOT NULL,
  resource_id      TEXT NOT NULL,
  revision_id      TEXT NOT NULL,
  context_space_id TEXT,
  classification   TEXT NOT NULL,
  purpose_allowlist TEXT[] NOT NULL DEFAULT '{}',
  retention_policy_id TEXT NOT NULL DEFAULT 'default',
  visibility_ref   TEXT NOT NULL,
  tags             TEXT[] NOT NULL DEFAULT '{}',
  content_locator  TEXT,
  title            TEXT,
  lifecycle_state  TEXT NOT NULL,
  idempotency_key  TEXT,
  attributes       JSONB NOT NULL DEFAULT '{}'::jsonb,
  valid_time       TIMESTAMPTZ,
  system_time      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, resource_id, revision_id),
  UNIQUE (organization_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_records_org_resource
  ON records (organization_id, resource_id);

CREATE TABLE IF NOT EXISTS evidence_refs (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  resource_id      TEXT NOT NULL,
  revision_id      TEXT,
  uri              TEXT NOT NULL,
  object_version   TEXT,
  sha256           TEXT,
  content_type     TEXT,
  byte_size        BIGINT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_evidence_org_resource
  ON evidence_refs (organization_id, resource_id);

CREATE TABLE IF NOT EXISTS derived_artifacts (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  resource_id      TEXT NOT NULL,
  revision_id      TEXT NOT NULL,
  kind             TEXT NOT NULL,
  uri              TEXT,
  content_hash     TEXT,
  confidence       DOUBLE PRECISION,
  review_state     TEXT,
  attributes       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_derived_org_resource
  ON derived_artifacts (organization_id, resource_id);

CREATE TABLE IF NOT EXISTS chunks (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  resource_id      TEXT NOT NULL,
  revision_id      TEXT NOT NULL,
  context_space_id TEXT,
  ordinal          INT NOT NULL DEFAULT 0,
  text             TEXT,
  locator          TEXT,
  redaction_profile TEXT,
  attributes       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_chunks_org_resource
  ON chunks (organization_id, resource_id);

-- Embeddings: BYTEA fallback when pgvector is unavailable.
-- When vector extension is present, operators may ALTER COLUMN embedding TYPE vector(...).
CREATE TABLE IF NOT EXISTS embeddings (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  chunk_id         TEXT NOT NULL,
  resource_id      TEXT NOT NULL,
  revision_id      TEXT NOT NULL,
  model            TEXT NOT NULL DEFAULT 'default',
  dims             INT NOT NULL DEFAULT 0,
  embedding        BYTEA NOT NULL, -- serialized float32 little-endian; pgvector optional
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_embeddings_org_resource
  ON embeddings (organization_id, resource_id);

CREATE TABLE IF NOT EXISTS sources (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL REFERENCES organizations(id),
  system           TEXT NOT NULL,
  display_name     TEXT,
  trust_tier       TEXT NOT NULL DEFAULT 'untrusted_external',
  mapping_spec_id  TEXT,
  enabled          BOOLEAN NOT NULL DEFAULT true,
  attributes       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE TABLE IF NOT EXISTS mapping_specs (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  version          TEXT NOT NULL,
  source_kind      TEXT NOT NULL,
  rules            JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id, version)
);

CREATE TABLE IF NOT EXISTS delegation_grants (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  subject_id       TEXT NOT NULL,
  actor_id         TEXT NOT NULL,
  owner_id         TEXT,
  actions          TEXT[] NOT NULL DEFAULT '{}',
  resource_ids     TEXT[] NOT NULL DEFAULT '{}',
  purposes         TEXT[] NOT NULL DEFAULT '{}',
  expires_at       TIMESTAMPTZ,
  budget           JSONB,
  revoked          BOOLEAN NOT NULL DEFAULT false,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE TABLE IF NOT EXISTS agent_credentials (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  agent_id         TEXT NOT NULL,
  owner_id         TEXT NOT NULL,
  secret_hash      TEXT NOT NULL,
  delegation_id    TEXT,
  expires_at       TIMESTAMPTZ,
  revoked          BOOLEAN NOT NULL DEFAULT false,
  label            TEXT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE TABLE IF NOT EXISTS outbox (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  subject          TEXT NOT NULL,
  payload          BYTEA NOT NULL,
  headers          JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at     TIMESTAMPTZ,
  PRIMARY KEY (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_outbox_unpublished
  ON outbox (organization_id, created_at)
  WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS consumer_receipts (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  consumer         TEXT NOT NULL,
  msg_id           TEXT NOT NULL,
  processed_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id),
  UNIQUE (organization_id, consumer, msg_id)
);

CREATE TABLE IF NOT EXISTS audit_events (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  principal_id     TEXT NOT NULL,
  principal_kind   TEXT,
  delegation_id    TEXT,
  action           TEXT NOT NULL,
  reason_code      TEXT NOT NULL,
  authz_model_rev  TEXT,
  policy_revision  TEXT,
  resource_count   INT NOT NULL DEFAULT 0,
  resource_ids_sample TEXT[] NOT NULL DEFAULT '{}',
  trace_id         TEXT,
  attributes       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_audit_org_created
  ON audit_events (organization_id, created_at DESC);

CREATE TABLE IF NOT EXISTS change_events (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  resource_id      TEXT NOT NULL,
  revision_id      TEXT,
  action           TEXT NOT NULL,
  cursor           TEXT NOT NULL,
  occurred_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_change_org_cursor
  ON change_events (organization_id, cursor);

CREATE TABLE IF NOT EXISTS webhook_subscriptions (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  target_url       TEXT NOT NULL,
  secret_hash      TEXT,
  events           TEXT[] NOT NULL DEFAULT '{}',
  enabled          BOOLEAN NOT NULL DEFAULT true,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  subscription_id  TEXT NOT NULL,
  event_id         TEXT NOT NULL,
  status           TEXT NOT NULL,
  attempts         INT NOT NULL DEFAULT 0,
  last_error       TEXT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  delivered_at     TIMESTAMPTZ,
  PRIMARY KEY (organization_id, id)
);

CREATE TABLE IF NOT EXISTS deletion_jobs (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  scope            JSONB NOT NULL DEFAULT '{}'::jsonb,
  status           TEXT NOT NULL DEFAULT 'pending',
  requested_by     TEXT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at     TIMESTAMPTZ,
  PRIMARY KEY (organization_id, id)
);

CREATE TABLE IF NOT EXISTS export_jobs (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  status           TEXT NOT NULL DEFAULT 'pending',
  manifest_uri     TEXT,
  requested_by     TEXT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at     TIMESTAMPTZ,
  PRIMARY KEY (organization_id, id)
);

CREATE TABLE IF NOT EXISTS quotas (
  organization_id  TEXT NOT NULL PRIMARY KEY REFERENCES organizations(id),
  search_per_minute INT NOT NULL DEFAULT 60,
  intake_per_minute INT NOT NULL DEFAULT 120,
  export_per_minute INT NOT NULL DEFAULT 10,
  max_results       INT NOT NULL DEFAULT 25,
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- FORCE ROW LEVEL SECURITY on all tenant tables.
DO $$
DECLARE
  t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'context_spaces','principals','resources','revisions','records','evidence_refs',
    'derived_artifacts','chunks','embeddings','sources','mapping_specs','delegation_grants',
    'agent_credentials','outbox','consumer_receipts','audit_events','change_events',
    'webhook_subscriptions','webhook_deliveries','deletion_jobs','export_jobs','quotas'
  ]
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format(
      'DROP POLICY IF EXISTS tenant_isolation ON %I', t);
    EXECUTE format(
      'CREATE POLICY tenant_isolation ON %I
         USING (organization_id = current_setting(''app.organization_id'', true))
         WITH CHECK (organization_id = current_setting(''app.organization_id'', true))', t);
    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO context_gateway', t);
  END LOOP;
END
$$;

GRANT SELECT, INSERT, UPDATE, DELETE ON organizations TO context_gateway;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO context_gateway;
