-- Legal holds block evidence purge until explicitly released.
CREATE TABLE IF NOT EXISTS legal_holds (
  organization_id TEXT NOT NULL,
  resource_id     TEXT NOT NULL,
  held            BOOLEAN NOT NULL DEFAULT true,
  reason          TEXT,
  created_by      TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  released_at     TIMESTAMPTZ,
  PRIMARY KEY (organization_id, resource_id)
);

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'context_gateway') THEN
    ALTER TABLE legal_holds ENABLE ROW LEVEL SECURITY;
    ALTER TABLE legal_holds FORCE ROW LEVEL SECURITY;
    DROP POLICY IF EXISTS legal_holds_org ON legal_holds;
    CREATE POLICY legal_holds_org ON legal_holds
      FOR ALL TO context_gateway
      USING (organization_id = current_setting('app.organization_id', true))
      WITH CHECK (organization_id = current_setting('app.organization_id', true));
    GRANT SELECT, INSERT, UPDATE, DELETE ON legal_holds TO context_gateway;
  END IF;
END
$$;
