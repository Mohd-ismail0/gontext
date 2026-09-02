#!/usr/bin/env bash
# Compose starter smoke — health/ready + optional OIDC token probe.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_DIR="$ROOT/deploy/compose"
ENV_FILE="${ENV_FILE:-$COMPOSE_DIR/.env.starter}"
COMPOSE_FILES="${COMPOSE_FILES:-$COMPOSE_DIR/docker-compose.yml $COMPOSE_DIR/docker-compose.starter.yaml}"
SMOKE_TIMEOUT="${SMOKE_TIMEOUT:-300}"
BASE_URL="${SMOKE_BASE_URL:-}"

compose() {
  local -a file_args=()
  local f
  for f in $COMPOSE_FILES; do
    file_args+=(-f "$f")
  done
  docker compose "${file_args[@]}" --env-file "$ENV_FILE" "$@"
}

if [[ ! -f "$ENV_FILE" ]]; then
  cp "$COMPOSE_DIR/.env.starter.example" "$ENV_FILE"
  echo "smoke: seeded $ENV_FILE from example (CI/lab only)"
fi

if [[ -n "${COMPOSE_TEST_OVERLAY:-}" ]]; then
  COMPOSE_FILES="$COMPOSE_FILES $COMPOSE_TEST_OVERLAY"
fi

echo "==> compose starter smoke (timeout ${SMOKE_TIMEOUT}s)"
if ! compose up -d --build; then
  echo "smoke: compose up failed; dumping one-shot service logs" >&2
  compose logs --tail=120 migrate bootstrap minio-init 2>/dev/null || true
  exit 1
fi

deadline=$((SECONDS + SMOKE_TIMEOUT))
ready=0
while (( SECONDS < deadline )); do
  if compose exec -T serve wget -qO- http://127.0.0.1:8080/health/ready >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 5
done

if [[ "$ready" -ne 1 ]]; then
  echo "smoke: /health/ready not green within ${SMOKE_TIMEOUT}s" >&2
  compose ps
  compose logs --tail=80 serve worker caddy
  exit 1
fi
echo "smoke: serve /health/ready ok"

if [[ -z "$BASE_URL" ]]; then
  # shellcheck disable=SC1090
  set -a && source "$ENV_FILE" && set +a
  BASE_URL="https://${PUBLIC_DOMAIN:-localhost}"
  if [[ "${PUBLIC_DOMAIN:-localhost}" == "localhost" ]]; then
    BASE_URL="http://localhost:8080"
    if compose ps serve 2>/dev/null | grep -q "8080"; then
      :
    else
      BASE_URL="http://localhost"
    fi
  fi
fi

echo "smoke: probing version via serve container"
if compose exec -T serve wget -qO- http://127.0.0.1:8080/v1/system/version | grep -q product_version; then
  echo "smoke: version endpoint ok"
elif curl -fsSk "$BASE_URL/v1/system/version" 2>/dev/null | grep -q product_version; then
  echo "smoke: version endpoint ok (via gateway $BASE_URL)"
else
  echo "smoke: version endpoint unreachable (gateway may still be provisioning TLS)" >&2
fi

# Optional Dex/static OIDC smoke when DEX_TOKEN_URL is set (integration CI).
if [[ -n "${DEX_TOKEN_URL:-}" ]]; then
  echo "smoke: fetching OIDC token from $DEX_TOKEN_URL"
  # Dex confidential clients authenticate via HTTP Basic, not body client_secret.
  token_json="$(curl -sS -X POST "$DEX_TOKEN_URL" \
    -u "${DEX_CLIENT_ID:-context-fabric-starter}:${DEX_CLIENT_SECRET:-}" \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode 'grant_type=password' \
    --data-urlencode "username=${DEX_USERNAME:-alice@example.com}" \
    --data-urlencode "password=${DEX_PASSWORD:-password}" \
    --data-urlencode 'scope=openid email groups' || true)"
  access_token="$(printf '%s' "$token_json" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('access_token') or d.get('id_token') or '')" 2>/dev/null || true)"
  if [[ -z "$access_token" ]]; then
    echo "smoke: failed to obtain access_token from Dex: ${token_json:-<empty>}" >&2
    exit 1
  fi
  http_code="$(curl -sSk -o /dev/null -w '%{http_code}' \
    -H "Authorization: Bearer $access_token" \
    "$BASE_URL/v1/organizations/org1/status" || true)"
  case "$http_code" in
    200|400|403|404) echo "smoke: OIDC-authenticated probe ok (HTTP $http_code)" ;;
    *)
      echo "smoke: authenticated org status returned HTTP ${http_code:-none} (token rejected or gateway down)" >&2
      exit 1
      ;;
  esac
fi

echo "smoke: passed"
