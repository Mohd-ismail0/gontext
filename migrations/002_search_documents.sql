-- Rebuildable search projection shared by serve/worker (INDEX_BACKEND=postgres).
-- Tags are AND-narrow filters only; they never grant ACL.

CREATE TABLE IF NOT EXISTS search_documents (
  organization_id  TEXT NOT NULL,
  resource_id      TEXT NOT NULL,
  revision_id      TEXT NOT NULL DEFAULT '',
  text             TEXT NOT NULL DEFAULT '',
  tags             TEXT[] NOT NULL DEFAULT '{}',
  attributes       JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, resource_id)
);

CREATE INDEX IF NOT EXISTS idx_search_documents_org
  ON search_documents (organization_id);

CREATE INDEX IF NOT EXISTS idx_search_documents_tags
  ON search_documents USING GIN (tags);

CREATE TABLE IF NOT EXISTS access_requests (
  id               TEXT NOT NULL,
  organization_id  TEXT NOT NULL,
  resource_id      TEXT NOT NULL,
  purpose          TEXT NOT NULL DEFAULT '',
  justification    TEXT NOT NULL DEFAULT '',
  status           TEXT NOT NULL DEFAULT 'pending',
  audit_id         TEXT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

DO $$
DECLARE
  t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY['search_documents', 'access_requests']
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
    EXECUTE format(
      'CREATE POLICY tenant_isolation ON %I
         USING (organization_id = current_setting(''app.organization_id'', true))
         WITH CHECK (organization_id = current_setting(''app.organization_id'', true))', t);
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'context_gateway') THEN
      EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO context_gateway', t);
    END IF;
  END LOOP;
END
$$;
