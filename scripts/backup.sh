#!/usr/bin/env bash
# Context Fabric host-managed backup — Postgres + multi-bucket evidence + OpenFGA notes.
# Run from an ops host or utility container with pg_dump and aws/mc installed.
set -euo pipefail

OUT_DIR="${1:-./backup-$(date -u +%Y%m%dT%H%M%SZ)}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

mkdir -p "$OUT_DIR/evidence/raw" "$OUT_DIR/evidence/derived" "$OUT_DIR/evidence/quarantine" "$OUT_DIR/openfga"

echo "==> backup bundle -> $OUT_DIR"

PRODUCT_VERSION="${CONTEXT_FABRIC_VERSION:-$(cat "$ROOT/VERSION" 2>/dev/null || echo unknown)}"
MIGRATION_HEAD="$(ls -1 "$ROOT/migrations"/*.sql 2>/dev/null | sort | tail -1 | xargs -n1 basename 2>/dev/null || echo unknown)"

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

sync_bucket() {
  local bucket="$1"
  local dest="$2"
  [[ -n "$bucket" ]] || return 0
  echo "==> evidence sync s3://${bucket} -> $dest"
  if command -v aws >/dev/null 2>&1; then
    aws s3 sync "s3://${bucket}" "$dest" --only-show-errors
  elif command -v mc >/dev/null 2>&1; then
    mc mirror --overwrite "local/${bucket}" "$dest"
  else
    echo "aws/mc not found; wrote sync instructions only"
    echo "aws s3 sync \"s3://${bucket}\" \"$dest\"" >"$dest/SYNC.txt"
  fi
}

RAW_BUCKET="${S3_BUCKET_RAW:-${S3_BUCKET:-}}"
DERIVED_BUCKET="${S3_BUCKET_DERIVED:-}"
QUAR_BUCKET="${S3_BUCKET_QUARANTINE:-}"

sync_bucket "$RAW_BUCKET" "$OUT_DIR/evidence/raw"
sync_bucket "$DERIVED_BUCKET" "$OUT_DIR/evidence/derived"
sync_bucket "$QUAR_BUCKET" "$OUT_DIR/evidence/quarantine"

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

checksums_file="$OUT_DIR/checksums.sha256"
: >"$checksums_file"
while IFS= read -r -d '' f; do
  rel="${f#"$OUT_DIR"/}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$f" | awk -v p="$rel" '{print $1 "  " p}' >>"$checksums_file"
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$f" | awk -v p="$rel" '{print $1 "  " p}' >>"$checksums_file"
  fi
done < <(find "$OUT_DIR" -type f ! -name 'checksums.sha256' -print0 2>/dev/null || true)

cat >"$OUT_DIR/manifest.json" <<EOF
{
  "format": "context-fabric.backup/v1",
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "product": "gontext",
  "product_version": "${PRODUCT_VERSION}",
  "migration_head": "${MIGRATION_HEAD}",
  "openfga_model_id": "${OPENFGA_MODEL_ID:-}",
  "openfga_store_id": "${OPENFGA_STORE_ID:-}",
  "includes": ["postgres.dump", "evidence/raw", "evidence/derived", "evidence/quarantine", "openfga/", "checksums.sha256"],
  "buckets": {
    "raw": "${RAW_BUCKET}",
    "derived": "${DERIVED_BUCKET}",
    "quarantine": "${QUAR_BUCKET}"
  }
}
EOF

echo "==> manifest + checksums written"
echo "==> done"
