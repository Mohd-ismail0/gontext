-- Sparse FTS projection + leased NATS outbox claims (ADR 0015 / production hot path).
-- Indexed content stays thin: IDs, title, kind, labels, short summary — not raw evidence.

ALTER TABLE search_documents
  ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS context_space_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '';

-- Materialized tsvector from thin fields only (never full evidence blobs).
ALTER TABLE search_documents
  ADD COLUMN IF NOT EXISTS search_tsv tsvector
  GENERATED ALWAYS AS (
    setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(summary, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(kind, '')), 'C') ||
    setweight(to_tsvector('english', coalesce(text, '')), 'D') ||
    setweight(to_tsvector('english', coalesce(array_to_string(tags, ' '), '')), 'C')
  ) STORED;

CREATE INDEX IF NOT EXISTS idx_search_documents_tsv
  ON search_documents USING GIN (search_tsv);

CREATE INDEX IF NOT EXISTS idx_search_documents_org_cs
  ON search_documents (organization_id, context_space_id);

-- Leased claim for event outbox so multiple workers do not race.
ALTER TABLE outbox
  ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_outbox_claim
  ON outbox (created_at)
  WHERE published_at IS NULL;

-- Two-arg form has NO default. A default plus a 1-arg wrapper makes
-- claim_outbox_batch($1) ambiguous in PostgreSQL.
CREATE OR REPLACE FUNCTION claim_outbox_batch(p_limit int, p_lease interval)
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

CREATE OR REPLACE FUNCTION mark_outbox_published_batch(p_ids text[], p_at timestamptz)
RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  n bigint;
BEGIN
  UPDATE outbox
  SET published_at = p_at, lease_until = NULL
  WHERE id = ANY(p_ids) AND published_at IS NULL;
  GET DIAGNOSTICS n = ROW_COUNT;
  RETURN n;
END;
$$;

-- Compatibility wrapper: claim_outbox_batch(int) -> default lease.
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
LANGUAGE sql
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT * FROM claim_outbox_batch(p_limit, interval '30 seconds');
$$;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'context_gateway') THEN
    GRANT EXECUTE ON FUNCTION claim_outbox_batch(int, interval) TO context_gateway;
    GRANT EXECUTE ON FUNCTION claim_outbox_batch(int) TO context_gateway;
  END IF;
END
$$;
