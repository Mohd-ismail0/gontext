#!/usr/bin/env bash
# Context Fabric host-managed restore — verify manifest/checksums, ledger, evidence, reconcile.
set -euo pipefail

IN_DIR="${1:-}"
if [[ -z "$IN_DIR" || ! -d "$IN_DIR" ]]; then
  echo "usage: $0 <backup-dir>" >&2
  exit 2
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> restore from $IN_DIR"

if [[ -f "$IN_DIR/manifest.json" ]]; then
  echo "==> manifest:"
  cat "$IN_DIR/manifest.json"
  if command -v python3 >/dev/null 2>&1; then
    fmt="$(python3 -c "import json; print(json.load(open('$IN_DIR/manifest.json')).get('format',''))")"
    if [[ "$fmt" != "context-fabric.backup/v1" ]]; then
      echo "warning: unexpected manifest format $fmt" >&2
    fi
  fi
else
  echo "warning: manifest.json missing — continuing (legacy bundle)" >&2
fi

if [[ -f "$IN_DIR/checksums.sha256" ]]; then
  echo "==> verifying checksums.sha256"
  if command -v sha256sum >/dev/null 2>&1; then
  (
    cd "$IN_DIR"
    sha256sum -c checksums.sha256
  )
  elif command -v shasum >/dev/null 2>&1; then
  (
    cd "$IN_DIR"
    shasum -a 256 -c checksums.sha256
  )
  else
    echo "warning: no sha256sum/shasum; skipping checksum verification" >&2
  fi
fi

DSN="${POSTGRES_ADMIN_DSN:-${POSTGRES_DSN:-}}"
if [[ -n "$DSN" && -f "$IN_DIR/postgres.dump" ]]; then
  echo "==> PostgreSQL pg_restore"
  pg_restore --clean --if-exists --no-owner --dbname="$DSN" "$IN_DIR/postgres.dump"
  echo "==> migrate forward (idempotent)"
  if command -v ./bin/context-fabric >/dev/null 2>&1; then
    ./bin/context-fabric migrate || true
  elif command -v context-fabric >/dev/null 2>&1; then
    context-fabric migrate || true
  else
    echo "warning: context-fabric binary not found; run migrate manually" >&2
  fi
else
  echo "Skipping pg_restore (need POSTGRES_DSN/ADMIN and postgres.dump)"
  if [[ -z "${CONTEXT_FABRIC_ALLOW_STUB_OPS:-}" ]]; then
    exit 2
  fi
fi

restore_bucket() {
  local src="$1"
  local bucket="$2"
  [[ -n "$bucket" && -d "$src" ]] || return 0
  echo "==> evidence restore $src -> s3://${bucket}"
  if command -v aws >/dev/null 2>&1; then
    aws s3 sync "$src" "s3://${bucket}"
  elif command -v mc >/dev/null 2>&1; then
    mc mirror --overwrite "$src" "local/${bucket}"
  else
    echo "aws/mc not found; restore evidence manually from $src"
  fi
}

RAW_BUCKET="${S3_BUCKET_RAW:-${S3_BUCKET:-}}"
DERIVED_BUCKET="${S3_BUCKET_DERIVED:-}"
QUAR_BUCKET="${S3_BUCKET_QUARANTINE:-}"

restore_bucket "$IN_DIR/evidence/raw" "$RAW_BUCKET"
restore_bucket "$IN_DIR/evidence/derived" "$DERIVED_BUCKET"
restore_bucket "$IN_DIR/evidence/quarantine" "$QUAR_BUCKET"

# Legacy single-bucket layout
if [[ -d "$IN_DIR/evidence" && ! -d "$IN_DIR/evidence/raw" && -n "${S3_BUCKET:-}" ]]; then
  restore_bucket "$IN_DIR/evidence" "${S3_BUCKET}"
fi

echo "==> OpenFGA: re-apply model + tuples from $IN_DIR/openfga before serving"
echo "CRITICAL: replay tombstones/revocations are in the ledger dump; do not serve"
echo "          until doctor is green and reconcile has run."

echo "==> reconcile AuthZ outbox / parent inheritance"
bash "$ROOT/scripts/reconcile.sh"

echo "==> doctor"
if command -v ./bin/context-fabric >/dev/null 2>&1; then
  ./bin/context-fabric doctor
elif command -v context-fabric >/dev/null 2>&1; then
  context-fabric doctor
else
  echo "warning: context-fabric binary not found; run doctor manually" >&2
fi

echo "==> done — safe to start serve/worker"
