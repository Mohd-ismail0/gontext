#!/usr/bin/env bash
# Context Fabric upgrade helper — pull pinned image, migrate, rolling restart serve/worker.
set -euo pipefail

COMPOSE_DIR="${COMPOSE_DIR:-deploy/compose}"
TARGET_IMAGE="${TARGET_IMAGE:-}"
ENV_FILE="${ENV_FILE:-${COMPOSE_DIR}/.env.starter}"
COMPOSE=(docker compose -f "${COMPOSE_DIR}/docker-compose.yml" -f "${COMPOSE_DIR}/docker-compose.starter.yaml" --env-file "${ENV_FILE}")

usage() {
  echo "usage: $0 [--image ghcr.io/mohd-ismail0/gontext@sha256:...] [--env-file path]" >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image) TARGET_IMAGE="$2"; shift 2 ;;
    --env-file) ENV_FILE="$2"; COMPOSE=(docker compose -f "${COMPOSE_DIR}/docker-compose.yml" -f "${COMPOSE_DIR}/docker-compose.starter.yaml" --env-file "${ENV_FILE}"); shift 2 ;;
    -h|--help) usage ;;
    *) echo "unknown arg: $1" >&2; usage ;;
  esac
done

if [[ -z "$TARGET_IMAGE" ]]; then
  echo "TARGET_IMAGE or --image is required (pin digest for production)" >&2
  exit 2
fi

echo "==> upgrade: pull ${TARGET_IMAGE}"
docker pull "$TARGET_IMAGE"

export GONTEXT_IMAGE="$TARGET_IMAGE"

echo "==> upgrade: migrate"
"${COMPOSE[@]}" run --rm migrate

echo "==> upgrade: restart worker (drain projections)"
"${COMPOSE[@]}" up -d --no-deps --force-recreate worker

echo "==> upgrade: restart serve"
"${COMPOSE[@]}" up -d --no-deps --force-recreate serve

echo "==> upgrade: doctor"
"${COMPOSE[@]}" run --rm --no-deps serve doctor

echo "==> upgrade: smoke (optional — set SMOKE_INSECURE=1 for self-signed Caddy)"
if [[ -f scripts/compose-smoke.sh ]]; then
  COMPOSE_FILE="${COMPOSE_DIR}/docker-compose.yml:${COMPOSE_DIR}/docker-compose.starter.yaml" \
    ENV_FILE="${ENV_FILE}" \
    bash scripts/compose-smoke.sh || true
fi

echo "==> upgrade: done"
