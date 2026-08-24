#!/usr/bin/env bash
# Context Fabric backup helper — Postgres ledger, evidence objects, OpenFGA notes.
set -euo pipefail

OUT_DIR="${1:-./backup-$(date -u +%Y%m%dT%H%M%SZ)}"
mkdir -p "$OUT_DIR/evidence" "$OUT_DIR/openfga"

echo "==> backup bundle -> $OUT_DIR"

if [[ -n "${POSTGRES_ADMIN_DSN:-}" ]]; then
  echo "==> PostgreSQL pg_dump (POSTGRES_ADMIN_DSN)"
  pg_dump --format=custom --file="$OUT_DIR/postgres.dump" "$POSTGRES_ADMIN_DSN"
elif [[ -n "${POSTGRES_DSN:-}" ]]; then
  echo "==> PostgreSQL pg_dump (POSTGRES_DSN; prefer POSTGRES_ADMIN_DSN for full backups)"
  pg_dump --format=custom --file="$OUT_DIR/postgres.dump" "$POSTGRES_DSN"
else
  echo "POSTGRES_ADMIN_DSN / POSTGRES_DSN unset; skipping pg_dump" >&2
  if [[ -z "${CONTEXT_FABRIC_ALLOW_STUB_OPS:-}" ]]; then
    exit 2
  fi
fi

if [[ -n "${S3_BUCKET:-}" ]]; then
  echo "==> evidence sync s3://${S3_BUCKET} -> $OUT_DIR/evidence"
  if command -v aws >/dev/null 2>&1; then
    aws s3 sync "s3://${S3_BUCKET}" "$OUT_DIR/evidence"
  elif command -v mc >/dev/null 2>&1; then
    mc mirror "local/${S3_BUCKET}" "$OUT_DIR/evidence"
  else
    echo "aws/mc not found; wrote sync instructions only"
    echo "aws s3 sync \"s3://${S3_BUCKET}\" \"$OUT_DIR/evidence\"" >"$OUT_DIR/evidence/SYNC.txt"
  fi
else
  echo "S3_BUCKET unset; skipping evidence sync"
fi

mkdir -p "$OUT_DIR/openfga"
cat >"$OUT_DIR/openfga/README.md" <<'EOF'
Place OpenFGA model + tuple export here. Example:
  fga store list
  fga model get --store-id "$OPENFGA_STORE_ID"
  fga tuple read --store-id "$OPENFGA_STORE_ID"
After restore: import tuples, then run scripts/reconcile.sh so sync_authz parents converge.
EOF

if [[ -n "${OPENFGA_API_URL:-}" && -n "${OPENFGA_STORE_ID:-}" ]] && command -v fga >/dev/null 2>&1; then
  echo "==> OpenFGA tuple sample export"
  fga tuple read --store-id "$OPENFGA_STORE_ID" --max-pages 5 \
    >"$OUT_DIR/openfga/tuples.jsonl" 2>/dev/null || true
fi

cat >"$OUT_DIR/manifest.json" <<EOF
{
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "product": "context-fabric",
  "includes": ["postgres.dump", "evidence/", "openfga/"]
}
EOF

echo "==> done"
