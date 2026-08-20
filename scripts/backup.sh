#!/usr/bin/env bash
# Context Fabric backup helper (documents the operator sequence).
# Requires: pg_dump, optional aws/mc CLI for object storage.
set -euo pipefail

OUT_DIR="${1:-./backup-$(date -u +%Y%m%dT%H%M%SZ)}"
mkdir -p "$OUT_DIR"

echo "==> backup bundle -> $OUT_DIR"

if [[ -n "${POSTGRES_DSN:-}" ]]; then
  echo "==> PostgreSQL pg_dump"
  pg_dump --format=custom --file="$OUT_DIR/postgres.dump" "$POSTGRES_DSN"
else
  echo "POSTGRES_DSN unset; skipping pg_dump (document DSN for production)"
fi

if [[ -n "${S3_BUCKET:-}" ]]; then
  echo "==> S3/MinIO sync placeholder"
  echo "Run: aws s3 sync \"s3://${S3_BUCKET}\" \"$OUT_DIR/evidence\" --delete"
  # aws s3 sync "s3://${S3_BUCKET}" "$OUT_DIR/evidence"
else
  echo "S3_BUCKET unset; skipping evidence sync"
fi

echo "==> OpenFGA export placeholder"
echo "Export store/model/tuples via OpenFGA CLI or control API into $OUT_DIR/openfga/"
mkdir -p "$OUT_DIR/openfga"
cat >"$OUT_DIR/openfga/README.md" <<'EOF'
Place OpenFGA model + tuple export here. Example (operator-run):
  fga store list
  fga model get --store-id ...
  fga tuple read --store-id ...
EOF

cat >"$OUT_DIR/manifest.json" <<EOF
{
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "product": "context-fabric",
  "includes": ["postgres.dump", "evidence/", "openfga/"]
}
EOF

echo "==> done"
