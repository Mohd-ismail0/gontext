#!/usr/bin/env bash
# Preflight checks for starter Compose overlay — validates .env.starter before deploy.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_DIR="$ROOT/deploy/compose"
ENV_FILE="${ENV_FILE:-$COMPOSE_DIR/.env.starter}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing $ENV_FILE — copy deploy/compose/.env.starter.example" >&2
  exit 2
fi

# shellcheck disable=SC1090
set -a
source "$ENV_FILE"
set +a

required=(
  GONTEXT_IMAGE
  CONTEXT_FABRIC_PROFILE
  POSTGRES_PASSWORD
  POSTGRES_GATEWAY_PASSWORD
  S3_ACCESS_KEY_ID
  S3_SECRET_ACCESS_KEY
  NATS_USER
  NATS_PASSWORD
  OPENFGA_API_TOKEN
  OIDC_ISSUER
  OIDC_AUDIENCE
  OIDC_CLIENT_ID
  OIDC_CLIENT_SECRET
  OIDC_DISCOVERY_URL
  OIDC_JWKS_URL
  WEBHOOK_SIGNING_SECRET
  DELETION_SIGNING_SECRET
  PUBLIC_DOMAIN
)

placeholders=(
  change-me
  replace-after
  example.com
  idp.example.com
)

fail=0
for key in "${required[@]}"; do
  val="${!key:-}"
  if [[ -z "$val" ]]; then
    echo "preflight: missing required $key" >&2
    fail=1
    continue
  fi
  for ph in "${placeholders[@]}"; do
    if [[ "$val" == *"$ph"* ]]; then
      echo "preflight: $key still contains placeholder ($ph)" >&2
      fail=1
    fi
  done
done

if [[ "${CONTEXT_FABRIC_PROFILE:-}" != "starter" ]]; then
  echo "preflight: CONTEXT_FABRIC_PROFILE must be starter (got ${CONTEXT_FABRIC_PROFILE:-unset})" >&2
  fail=1
fi

if [[ -n "${GONTEXT_IMAGE_DIGEST:-}" ]]; then
  if [[ "$GONTEXT_IMAGE" != *"@"* ]]; then
    echo "preflight: GONTEXT_IMAGE_DIGEST set but GONTEXT_IMAGE is not digest-pinned" >&2
    fail=1
  fi
fi

if command -v docker >/dev/null 2>&1; then
  if ! docker compose version >/dev/null 2>&1; then
    echo "preflight: docker compose plugin required" >&2
    fail=1
  fi
else
  echo "preflight: docker not found in PATH" >&2
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  exit 2
fi

echo "preflight: starter env ok ($ENV_FILE)"
echo "preflight: image=${GONTEXT_IMAGE} profile=${CONTEXT_FABRIC_PROFILE} domain=${PUBLIC_DOMAIN}"
