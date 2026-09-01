-- Context Fabric PostgreSQL bootstrap roles
-- Applied via docker-entrypoint-initdb.d on bundled Postgres images.
-- Runtime application connections must use context_gateway (NOBYPASSRLS).

-- Ensure we are connected as a superuser during init (default for official images).
SELECT current_user;

-- Application role: non-owner, cannot bypass row-level security.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'context_gateway') THEN
    -- Default password is set by deploy/postgres/init/02_gateway_password.sh from
    -- POSTGRES_GATEWAY_PASSWORD (must match deploy/compose/.env.example and app POSTGRES_DSN).
    CREATE ROLE context_gateway
      LOGIN
      NOSUPERUSER
      NOCREATEDB
      NOCREATEROLE
      NOINHERIT
      NOREPLICATION
      NOBYPASSRLS
      CONNECTION LIMIT 100
      PASSWORD 'change-me-postgres-gateway';
  ELSE
    ALTER ROLE context_gateway
      NOSUPERUSER
      NOCREATEDB
      NOCREATEROLE
      NOINHERIT
      NOREPLICATION
      NOBYPASSRLS;
  END IF;
END
$$;

COMMENT ON ROLE context_gateway IS
  'Context Fabric runtime role. MUST remain NOBYPASSRLS. Application code sets app.organization_id via set_config(..., true) per transaction after token validation.';

-- Harden public schema defaults for new objects.
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO context_gateway;

-- Future migrations should:
--   1. CREATE SCHEMA IF NOT EXISTS context AUTHORIZATION <migration_owner>;
--   2. REVOKE ALL ON SCHEMA context FROM PUBLIC;
--   3. GRANT USAGE ON SCHEMA context TO context_gateway;
--   4. For every protected table:
--        ALTER TABLE ... ENABLE ROW LEVEL SECURITY;
--        ALTER TABLE ... FORCE ROW LEVEL SECURITY;  -- required so table owners cannot skip RLS
--        REVOKE ALL ON TABLE ... FROM PUBLIC;
--        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE ... TO context_gateway;
--        CREATE POLICY ... TO context_gateway USING (organization_id = current_setting('app.organization_id', true))
--          WITH CHECK (organization_id = current_setting('app.organization_id', true));
--
-- FORCE RLS is intentionally applied in schema migrations (not here) once tables exist.
-- Doctor verifies: role has NOBYPASSRLS, protected tables have FORCE ROW LEVEL SECURITY,
-- and missing tenant context fails closed.

-- Optional migrate/bootstrap admin is the image superuser (POSTGRES_USER during init).
-- Do not grant BYPASSRLS or SUPERUSER to context_gateway under any profile.

-- Separate database for bundled OpenFGA datastore (do not colocate with ledger tables).
-- Runs only on first volume init (docker-entrypoint-initdb.d).
CREATE DATABASE openfga;
