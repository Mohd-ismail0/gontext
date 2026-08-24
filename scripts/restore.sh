#!/usr/bin/env bash
# Context Fabric restore helper — ledger first, evidence, OpenFGA, then reconcile + tombstone dominance.
set -euo pipefail

IN_DIR="${1:-}"
if [[ -z "$IN_DIR" || ! -d "$IN_DIR" ]]; then
  echo "usage: $0 <backup-dir>" >&2
  exit 2
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> restore from $IN_DIR"

DSN="${POSTGRES_ADMIN_DSN:-${POSTGRES_DSN:-}}"
if [[ -n "$DSN" && -f "$IN_DIR/postgres.dump" ]]; then
  echo "==> PostgreSQL pg_restore"
  pg_restore --clean --if-exists --no-owner --dbname="$DSN" "$IN_DIR/postgres.dump"
  echo "==> migrate forward (idempotent)"
  ./bin/context-fabric migrate || true
else
  echo "Skipping pg_restore (need POSTGRES_DSN/ADMIN and postgres.dump)"
  if [[ -z "${CONTEXT_FABRIC_ALLOW_STUB_OPS:-}" ]]; then
    exit 2
  fi
fi

if [[ -n "${S3_BUCKET:-}" && -d "$IN_DIR/evidence" ]]; then
  echo "==> evidence restore -> s3://${S3_BUCKET}"
  if command -v aws >/dev/null 2>&1; then
    aws s3 sync "$IN_DIR/evidence" "s3://${S3_BUCKET}"
  elif command -v mc >/dev/null 2>&1; then
    mc mirror "$IN_DIR/evidence" "local/${S3_BUCKET}"
  else
    echo "aws/mc not found; restore evidence manually from $IN_DIR/evidence"
  fi
fi

echo "==> OpenFGA: re-apply model + tuples from $IN_DIR/openfga before serving"
echo "CRITICAL: replay tombstones/revocations are in the ledger dump; do not serve"
echo "          until doctor is green and reconcile has run."

echo "==> reconcile AuthZ outbox / parent inheritance"
bash "$ROOT/scripts/reconcile.sh"

echo "==> doctor"
./bin/context-fabric doctor

echo "==> done — safe to start serve/worker"
