#!/bin/bash
set -euo pipefail

# Sets context_gateway password from POSTGRES_GATEWAY_PASSWORD.
# Must match POSTGRES_GATEWAY_PASSWORD in deploy/compose/.env and the password
# embedded in POSTGRES_DSN for serve/worker/migrate roles.
GATEWAY_PASSWORD="${POSTGRES_GATEWAY_PASSWORD:-change-me-postgres-gateway}"

psql -v ON_ERROR_STOP=1 \
  --username "${POSTGRES_USER}" \
  --dbname "${POSTGRES_DB}" \
  -v password="${GATEWAY_PASSWORD}" <<'EOSQL'
ALTER ROLE context_gateway PASSWORD :'password';
EOSQL
