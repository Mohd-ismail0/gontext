-- Knowledge-graph edges (ADR 0013).
-- Edges are organizational facts, not authorization grants.
-- AuthZ inheritance uses OpenFGA parent tuples separately.

CREATE TABLE IF NOT EXISTS graph_edges (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL REFERENCES organizations(id),
  from_id          TEXT NOT NULL,
  to_id            TEXT NOT NULL,
  predicate        TEXT NOT NULL,
  confidence       DOUBLE PRECISION NOT NULL DEFAULT 1.0,
  lifecycle_state  TEXT NOT NULL DEFAULT 'ACTIVE',
  attributes       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id),
  FOREIGN KEY (organization_id, from_id) REFERENCES resources (organization_id, id),
  FOREIGN KEY (organization_id, to_id) REFERENCES resources (organization_id, id),
  CONSTRAINT graph_edges_no_self CHECK (from_id <> to_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_graph_edges_active_triple
  ON graph_edges (organization_id, from_id, to_id, predicate)
  WHERE lifecycle_state = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_graph_edges_from
  ON graph_edges (organization_id, from_id)
  WHERE lifecycle_state = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_graph_edges_to
  ON graph_edges (organization_id, to_id)
  WHERE lifecycle_state = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_graph_edges_predicate
  ON graph_edges (organization_id, predicate)
  WHERE lifecycle_state = 'ACTIVE';

DO $$
BEGIN
  ALTER TABLE graph_edges ENABLE ROW LEVEL SECURITY;
  ALTER TABLE graph_edges FORCE ROW LEVEL SECURITY;
  DROP POLICY IF EXISTS tenant_isolation ON graph_edges;
  CREATE POLICY tenant_isolation ON graph_edges
    USING (organization_id = current_setting('app.organization_id', true))
    WITH CHECK (organization_id = current_setting('app.organization_id', true));
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'context_gateway') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON graph_edges TO context_gateway;
  END IF;
END
$$;
