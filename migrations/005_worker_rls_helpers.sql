-- Cross-tenant worker helpers via SECURITY DEFINER (NOBYPASSRLS on context_gateway).
-- Industry pattern: privileged claim/mark/count functions owned by migration role;
-- app role only EXECUTE. Request path continues to use SET LOCAL app.organization_id.

-- Read unpublished outbox rows across tenants (NATS Msg-Id provides publish idempotency).
-- Locks are not held across round-trips; AuthZ outbox uses durable lease_until instead.
CREATE OR REPLACE FUNCTION claim_outbox_batch(p_limit int)
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
  RETURN QUERY
  SELECT o.id, o.organization_id, o.subject, o.payload, o.headers, o.created_at, o.published_at
  FROM outbox o
  WHERE o.published_at IS NULL
  ORDER BY o.created_at ASC
  LIMIT p_limit;
END;
$$;

CREATE OR REPLACE FUNCTION mark_outbox_published_batch(p_ids text[], p_at timestamptz)
RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  n bigint;
BEGIN
  UPDATE outbox SET published_at = p_at WHERE id = ANY(p_ids) AND published_at IS NULL;
  GET DIAGNOSTICS n = ROW_COUNT;
  RETURN n;
END;
$$;

CREATE OR REPLACE FUNCTION claim_authz_tuple_outbox(p_limit int, p_lease interval)
RETURNS TABLE (
  id text,
  organization_id text,
  operation text,
  object text,
  relation text,
  subject text,
  edge_id text,
  status text,
  attempts int,
  last_error text,
  lease_until timestamptz,
  next_attempt timestamptz,
  created_at timestamptz,
  updated_at timestamptz,
  applied_at timestamptz
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF p_limit IS NULL OR p_limit < 1 THEN
    p_limit := 20;
  END IF;
  IF p_lease IS NULL THEN
    p_lease := interval '30 seconds';
  END IF;
  RETURN QUERY
  UPDATE authz_tuple_outbox AS t
  SET lease_until = now() + p_lease, updated_at = now()
  WHERE t.id IN (
    SELECT a.id FROM authz_tuple_outbox a
    WHERE a.status = 'pending'
      AND a.next_attempt <= now()
      AND (a.lease_until IS NULL OR a.lease_until < now())
    ORDER BY a.next_attempt ASC
    LIMIT p_limit
    FOR UPDATE SKIP LOCKED
  )
  RETURNING t.id, t.organization_id, t.operation, t.object, t.relation, t.subject,
            COALESCE(t.edge_id, ''), t.status, t.attempts, COALESCE(t.last_error, ''),
            t.lease_until, t.next_attempt, t.created_at, t.updated_at, t.applied_at;
END;
$$;

CREATE OR REPLACE FUNCTION mark_authz_tuple_applied(p_id text, p_at timestamptz)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  n int;
BEGIN
  UPDATE authz_tuple_outbox
  SET status = 'applied', applied_at = p_at, lease_until = NULL, last_error = '', updated_at = p_at
  WHERE id = p_id;
  GET DIAGNOSTICS n = ROW_COUNT;
  RETURN n > 0;
END;
$$;

CREATE OR REPLACE FUNCTION mark_authz_tuple_failed(
  p_id text, p_attempts int, p_next timestamptz, p_err text, p_dead boolean
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  n int;
  st text := 'pending';
BEGIN
  IF p_dead THEN
    st := 'dead';
  END IF;
  UPDATE authz_tuple_outbox
  SET status = st, attempts = p_attempts, next_attempt = p_next,
      last_error = p_err, lease_until = NULL, updated_at = now()
  WHERE id = p_id;
  GET DIAGNOSTICS n = ROW_COUNT;
  RETURN n > 0;
END;
$$;

CREATE OR REPLACE FUNCTION count_authz_tuple_outbox(p_org text)
RETURNS TABLE (pending bigint, dead bigint)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  RETURN QUERY
  SELECT
    COUNT(*) FILTER (WHERE status = 'pending')::bigint,
    COUNT(*) FILTER (WHERE status = 'dead')::bigint
  FROM authz_tuple_outbox
  WHERE p_org IS NULL OR p_org = '' OR organization_id = p_org;
END;
$$;

CREATE OR REPLACE FUNCTION has_authz_tuple_coverage(
  p_org text, p_operation text, p_object text, p_relation text, p_subject text
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  n int;
  op text := COALESCE(NULLIF(p_operation, ''), 'write');
BEGIN
  SELECT COUNT(*) INTO n FROM authz_tuple_outbox
  WHERE organization_id = p_org
    AND operation = op
    AND object = p_object
    AND relation = p_relation
    AND subject = p_subject
    AND status IN ('pending', 'applied');
  RETURN n > 0;
END;
$$;

CREATE OR REPLACE FUNCTION has_authz_tuple_pending(
  p_org text, p_operation text, p_object text, p_relation text, p_subject text
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  n int;
  op text := COALESCE(NULLIF(p_operation, ''), 'write');
BEGIN
  SELECT COUNT(*) INTO n FROM authz_tuple_outbox
  WHERE organization_id = p_org
    AND operation = op
    AND object = p_object
    AND relation = p_relation
    AND subject = p_subject
    AND status = 'pending';
  RETURN n > 0;
END;
$$;

CREATE OR REPLACE FUNCTION list_active_parent_edges_needing_authz(p_org text, p_limit int)
RETURNS TABLE (
  id text,
  organization_id text,
  from_id text,
  to_id text,
  predicate text,
  confidence double precision,
  lifecycle_state text,
  sync_authz boolean,
  attributes jsonb,
  created_at timestamptz,
  updated_at timestamptz
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF p_limit IS NULL OR p_limit < 1 THEN
    p_limit := 100;
  END IF;
  RETURN QUERY
  SELECT e.id, e.organization_id, e.from_id, e.to_id, e.predicate, e.confidence,
         e.lifecycle_state, COALESCE(e.sync_authz, false), e.attributes, e.created_at, e.updated_at
  FROM graph_edges e
  WHERE e.lifecycle_state = 'ACTIVE'
    AND e.predicate = 'parent'
    AND e.sync_authz = true
    AND (p_org IS NULL OR p_org = '' OR e.organization_id = p_org)
  ORDER BY e.created_at ASC
  LIMIT p_limit;
END;
$$;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'context_gateway') THEN
    GRANT EXECUTE ON FUNCTION claim_outbox_batch(int) TO context_gateway;
    GRANT EXECUTE ON FUNCTION mark_outbox_published_batch(text[], timestamptz) TO context_gateway;
    GRANT EXECUTE ON FUNCTION claim_authz_tuple_outbox(int, interval) TO context_gateway;
    GRANT EXECUTE ON FUNCTION mark_authz_tuple_applied(text, timestamptz) TO context_gateway;
    GRANT EXECUTE ON FUNCTION mark_authz_tuple_failed(text, int, timestamptz, text, boolean) TO context_gateway;
    GRANT EXECUTE ON FUNCTION count_authz_tuple_outbox(text) TO context_gateway;
    GRANT EXECUTE ON FUNCTION has_authz_tuple_coverage(text, text, text, text, text) TO context_gateway;
    GRANT EXECUTE ON FUNCTION has_authz_tuple_pending(text, text, text, text, text) TO context_gateway;
    GRANT EXECUTE ON FUNCTION list_active_parent_edges_needing_authz(text, int) TO context_gateway;
  END IF;
END
$$;

REVOKE ALL ON FUNCTION claim_outbox_batch(int) FROM PUBLIC;
REVOKE ALL ON FUNCTION mark_outbox_published_batch(text[], timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION claim_authz_tuple_outbox(int, interval) FROM PUBLIC;
REVOKE ALL ON FUNCTION mark_authz_tuple_applied(text, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION mark_authz_tuple_failed(text, int, timestamptz, text, boolean) FROM PUBLIC;
REVOKE ALL ON FUNCTION count_authz_tuple_outbox(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION has_authz_tuple_coverage(text, text, text, text, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION has_authz_tuple_pending(text, text, text, text, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION list_active_parent_edges_needing_authz(text, int) FROM PUBLIC;
