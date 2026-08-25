-- Durable AuthZ tuple outbox (ADR 0014) + placeholder lifecycle support.

CREATE TABLE IF NOT EXISTS authz_tuple_outbox (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL REFERENCES organizations(id),
  operation        TEXT NOT NULL CHECK (operation IN ('write','delete')),
  object           TEXT NOT NULL,
  relation         TEXT NOT NULL,
  subject          TEXT NOT NULL,
  edge_id          TEXT,
  status           TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','applied','dead')),
  attempts         INT NOT NULL DEFAULT 0,
  last_error       TEXT,
  lease_until      TIMESTAMPTZ,
  next_attempt     TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_at       TIMESTAMPTZ,
  PRIMARY KEY (organization_id, id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_authz_tuple_outbox_active
  ON authz_tuple_outbox (organization_id, operation, object, relation, subject)
  WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_authz_tuple_outbox_claim
  ON authz_tuple_outbox (status, next_attempt, lease_until)
  WHERE status = 'pending';

-- Mark knowledge parent edges that require AuthZ inheritance sync.
ALTER TABLE graph_edges
  ADD COLUMN IF NOT EXISTS sync_authz BOOLEAN NOT NULL DEFAULT false;

DO $$
BEGIN
  ALTER TABLE authz_tuple_outbox ENABLE ROW LEVEL SECURITY;
  ALTER TABLE authz_tuple_outbox FORCE ROW LEVEL SECURITY;
  DROP POLICY IF EXISTS tenant_isolation ON authz_tuple_outbox;
  CREATE POLICY tenant_isolation ON authz_tuple_outbox
    USING (organization_id = current_setting('app.organization_id', true))
    WITH CHECK (organization_id = current_setting('app.organization_id', true));
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'context_gateway') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON authz_tuple_outbox TO context_gateway;
  END IF;
END
$$;
