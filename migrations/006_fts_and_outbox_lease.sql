-- Sparse FTS projection + leased NATS outbox claims (ADR 0015 / production hot path).
-- Indexed content stays thin: IDs, title, kind, labels, short summary — not raw evidence.

ALTER TABLE search_documents
  ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS context_space_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '';

-- Materialized tsvector from thin fields only (never full evidence blobs).
-- Trigger-maintained: GENERATED STORED requires immutable expressions;
-- to_tsvector(regconfig, text) is STABLE in PostgreSQL.
ALTER TABLE search_documents
  ADD COLUMN IF NOT EXISTS search_tsv tsvector;

CREATE OR REPLACE FUNCTION search_documents_compute_tsv(
  p_title text,
  p_summary text,
  p_kind text,
  p_body text,
  p_tags text[]
) RETURNS tsvector
LANGUAGE sql
STABLE
SET search_path = public
AS $$
  SELECT
    setweight(to_tsvector('english', coalesce(p_title, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(p_summary, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(p_kind, '')), 'C') ||
    setweight(to_tsvector('english', coalesce(p_body, '')), 'D') ||
    setweight(to_tsvector('english', coalesce(array_to_string(p_tags, ' '), '')), 'C');
$$;

CREATE OR REPLACE FUNCTION search_documents_tsv_trigger()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public
AS $$
BEGIN
  NEW.search_tsv := search_documents_compute_tsv(
    NEW.title, NEW.summary, NEW.kind, NEW.text, NEW.tags
  );
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_search_documents_tsv ON search_documents;
CREATE TRIGGER trg_search_documents_tsv
  BEFORE INSERT OR UPDATE OF title, kind, context_space_id, summary, text, tags
  ON search_documents
  FOR EACH ROW
  EXECUTE FUNCTION search_documents_tsv_trigger();

UPDATE search_documents sd
SET search_tsv = search_documents_compute_tsv(sd.title, sd.summary, sd.kind, sd.text, sd.tags)
WHERE sd.search_tsv IS NULL;

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
