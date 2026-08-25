-- Durable credential resolution + webhook signing material (production v1 data plane).

CREATE INDEX IF NOT EXISTS idx_agent_credentials_secret_hash
  ON agent_credentials (secret_hash)
  WHERE revoked = false;

-- Cross-tenant API-key resolve (NOBYPASSRLS-safe SECURITY DEFINER).
CREATE OR REPLACE FUNCTION resolve_agent_credential(p_hash text)
RETURNS TABLE (
  organization_id text,
  id text,
  agent_id text,
  owner_id text,
  expires_at timestamptz,
  revoked boolean
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  RETURN QUERY
  SELECT c.organization_id, c.id, c.agent_id, c.owner_id, c.expires_at, c.revoked
  FROM agent_credentials c
  WHERE c.secret_hash = p_hash
  LIMIT 1;
END;
$$;

CREATE OR REPLACE FUNCTION revoke_agent_credential(p_id text)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE n int;
BEGIN
  UPDATE agent_credentials SET revoked = true WHERE id = p_id;
  GET DIAGNOSTICS n = ROW_COUNT;
  RETURN n > 0;
END;
$$;

-- Per-subscription signing secret (HMAC key material; not a public hash).
ALTER TABLE webhook_subscriptions
  ADD COLUMN IF NOT EXISTS signing_secret TEXT NOT NULL DEFAULT '';

ALTER TABLE webhook_deliveries
  ADD COLUMN IF NOT EXISTS next_attempt TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS payload_sha256 TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS status_code INT NOT NULL DEFAULT 0;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'context_gateway') THEN
    GRANT EXECUTE ON FUNCTION resolve_agent_credential(text) TO context_gateway;
    GRANT EXECUTE ON FUNCTION revoke_agent_credential(text) TO context_gateway;
  END IF;
END
$$;

REVOKE ALL ON FUNCTION resolve_agent_credential(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION revoke_agent_credential(text) FROM PUBLIC;
