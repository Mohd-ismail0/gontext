#!/usr/bin/env bash
# Context Fabric restore helper (documents the operator sequence).
set -euo pipefail

IN_DIR="${1:-}"
if [[ -z "$IN_DIR" || ! -d "$IN_DIR" ]]; then
  echo "usage: $0 <backup-dir>" >&2
  exit 2
fi

echo "==> restore from $IN_DIR"

if [[ -n "${POSTGRES_DSN:-}" && -f "$IN_DIR/postgres.dump" ]]; then
  echo "==> PostgreSQL pg_restore"
  pg_restore --clean --if-exists --no-owner --dbname="$POSTGRES_DSN" "$IN_DIR/postgres.dump" || true
else
  echo "Skipping pg_restore (need POSTGRES_DSN and postgres.dump)"
fi

if [[ -n "${S3_BUCKET:-}" && -d "$IN_DIR/evidence" ]]; then
  echo "==> S3/MinIO restore placeholder"
  echo "Run: aws s3 sync \"$IN_DIR/evidence\" \"s3://${S3_BUCKET}\""
fi

echo "==> OpenFGA import placeholder"
echo "Re-apply model + tuples from $IN_DIR/openfga before serving traffic"

echo "==> CRITICAL: replay tombstones/revocations before setting ready=true"
echo "Then: context-fabric migrate && context-fabric doctor && open serve"

echo "==> done"
