-- Repair ambiguous claim_outbox_batch overloads (006) and org-scope credential revoke (007).
-- Safe to re-run: DROP IF EXISTS then recreate.

DROP FUNCTION IF EXISTS claim_outbox_batch(int);
DROP FUNCTION IF EXISTS claim_outbox_batch(int, interval);

CREATE FUNCTION claim_outbox_batch(p_limit int, p_lease interval)
RETURNS TABLE (
  id text,
  organization_id text,
  subject text,
  payload bytea,
  headers jsonb,
  created_at timestamptz,
  published_at timestamptz
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF p_limit IS NULL OR p_limit < 1 THEN
    p_limit := 10;
  END IF;
  IF p_lease IS NULL THEN
    p_lease := interval '30 seconds';
  END IF;
  RETURN QUERY
  UPDATE outbox AS o
  SET lease_until = now() + p_lease
  WHERE o.id IN (
    SELECT x.id FROM outbox x
    WHERE x.published_at IS NULL
      AND (x.lease_until IS NULL OR x.lease_until < now())
    ORDER BY x.created_at ASC
    LIMIT p_limit
    FOR UPDATE SKIP LOCKED
  )
  RETURNING o.id, o.organization_id, o.subject, o.payload, o.headers, o.created_at, o.published_at;
END;
$$;

CREATE FUNCTION claim_outbox_batch(p_limit int)
RETURNS TABLE (
  id text,
  organization_id text,
  subject text,
  payload bytea,
  headers jsonb,
  created_at timestamptz,
  published_at timestamptz
)
LANGUAGE sql
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT * FROM claim_outbox_batch(p_limit, interval '30 seconds');
$$;

DROP FUNCTION IF EXISTS revoke_agent_credential(text);
DROP FUNCTION IF EXISTS revoke_agent_credential(text, text);

CREATE FUNCTION revoke_agent_credential(p_org text, p_id text)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE n int;
BEGIN
  IF p_org IS NULL OR p_org = '' OR p_id IS NULL OR p_id = '' THEN
    RETURN false;
  END IF;
  UPDATE agent_credentials
  SET revoked = true
  WHERE organization_id = p_org AND id = p_id;
  GET DIAGNOSTICS n = ROW_COUNT;
  RETURN n > 0;
END;
$$;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'context_gateway') THEN
    GRANT EXECUTE ON FUNCTION claim_outbox_batch(int, interval) TO context_gateway;
    GRANT EXECUTE ON FUNCTION claim_outbox_batch(int) TO context_gateway;
    GRANT EXECUTE ON FUNCTION revoke_agent_credential(text, text) TO context_gateway;
  END IF;
END
$$;

REVOKE ALL ON FUNCTION claim_outbox_batch(int) FROM PUBLIC;
REVOKE ALL ON FUNCTION claim_outbox_batch(int, interval) FROM PUBLIC;
REVOKE ALL ON FUNCTION revoke_agent_credential(text, text) FROM PUBLIC;
