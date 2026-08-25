#!/usr/bin/env bash
# Context Fabric AuthZ / index reconcile helper.
# Re-enqueues missing OpenFGA parent tuples for active sync_authz edges and
# optionally drains the AuthZ outbox via a short-lived worker process.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> reconcile AuthZ inheritance + projections"

if [[ -n "${CONTEXT_FABRIC_MEMORY:-}" || "${PROFILE:-}" == "demo" ]]; then
  if [[ -z "${CONTEXT_FABRIC_ALLOW_STUB_OPS:-}" ]]; then
    echo "memory/demo: set CONTEXT_FABRIC_ALLOW_STUB_OPS=1 to acknowledge in-process reconcile"
    exit 0
  fi
  echo "memory/demo: starting short-lived worker for AuthZ drain"
  CONTEXT_FABRIC_MEMORY=1 timeout 15s ./bin/context-fabric worker || true
  echo "==> done (memory)"
  exit 0
fi

if [[ -z "${POSTGRES_DSN:-}" && -z "${POSTGRES_ADMIN_DSN:-}" ]]; then
  echo "POSTGRES_DSN (or POSTGRES_ADMIN_DSN) required for reconcile" >&2
  exit 2
fi

echo "==> ensuring migrations (008+ outbox claim disambiguation, org-scoped revoke)"
./bin/context-fabric migrate

echo "==> short-lived worker drain (AuthZ outbox + index projection)"
# Worker reconcileLoop re-enqueues missing parent tuples; Drain is continuous —
# run briefly then stop.
timeout "${RECONCILE_SECONDS:-30}" ./bin/context-fabric worker || true

echo "==> doctor"
./bin/context-fabric doctor || true

echo "==> done"
echo "NOTE: if OpenFGA was restored from backup, run this after ledger restore so"
echo "      sync_authz parent edges re-converge (ADR 0014)."
