# Deploy Context Fabric — starter Compose (production)

Single-host Docker Compose deployment for the **starter** profile: bundled Postgres, MinIO, NATS, and OpenFGA; **external OIDC**; split `serve` + `worker`; Caddy TLS on ports 80/443 only.

For local development with Local auth and role `all`, use the **demo** stack instead (`make compose-up`).

## Prerequisites

- Docker Engine 24+ with Compose v2 plugin
- A reachable OIDC IdP (issuer, audience, discovery/JWKS, client credentials)
- DNS `A`/`AAAA` for `PUBLIC_DOMAIN` pointing at the host (or lab `localhost` without TLS)
- Host tools for backup/restore: `pg_dump`, `pg_restore`, `aws` or `mc` (see [backup-restore.md](./backup-restore.md))

## Quick start

```bash
cp deploy/compose/.env.starter.example deploy/compose/.env.starter
# Edit every secret and OIDC URL — preflight rejects placeholders.

make compose-starter-preflight
make compose-starter-up
```

Stack files:

```text
deploy/compose/docker-compose.yml          # base infra + jobs
deploy/compose/docker-compose.starter.yaml # production overlay
deploy/compose/.env.starter                # operator secrets (never commit)
deploy/compose/Caddyfile                   # TLS reverse proxy
```

## Architecture

| Surface | Exposure |
|---------|----------|
| Caddy (`caddy`) | **Public** — ports 80/443 only |
| `serve` | Private `data` + `gateway` networks; no host port |
| `worker` | Private `data` network only |
| Postgres, MinIO, NATS, OpenFGA | Private `data` network (`internal: true`) |

Roles:

- `migrate` → `bootstrap` → `serve` (healthy) → `worker`
- `INDEX_BACKEND=postgres` (durable index + AuthZ outbox)
- No `all` role in this overlay

Auth:

- `CONTEXT_FABRIC_PROFILE=starter` — Local/demo tokens rejected
- OpenFGA `preshared` auth (`OPENFGA_API_TOKEN` / `OPENFGA_AUTHN_PRESHARED_KEYS`)
- NATS user/password (`NATS_USER`, `NATS_PASSWORD`)
- `WEBHOOK_SIGNING_SECRET` and `DELETION_SIGNING_SECRET` required

## Image promotion

Production must pin an **immutable digest** published by the release workflow:

```bash
# After tagging v1.0.0 and CI release job completes:
GONTEXT_IMAGE=ghcr.io/mohd-ismail0/gontext@sha256:<digest>
```

Semver tags are pointers only. Verify cosign signature and SBOM before promote (see [release-gates.md](./release-gates.md)).

## Operations

| Task | Command |
|------|---------|
| Preflight | `make compose-starter-preflight` |
| Smoke test | `make compose-starter-smoke` |
| Stop | `make compose-starter-down` |
| Doctor | `docker compose ... run --rm serve doctor` |
| Backup (host) | `make backup OUT_DIR=./backups/$(date -u +%Y%m%d)` |
| Restore (host) | `make restore IN_DIR=./backups/<dir>` |
| Release gates | `make release-check` |

Backup bundles include `manifest.json`, `checksums.sha256`, Postgres dump, and per-bucket evidence sync (`raw`, `derived`, `quarantine`). Restore verifies checksums before replaying ledger and running `scripts/reconcile.sh`.

## Troubleshooting

1. **`make compose-starter-preflight` fails** — replace every `change-me` / `example.com` placeholder in `.env.starter`.
2. **`/health/ready` not green** — check `bootstrap` logs for OpenFGA store id; confirm `OPENFGA_API_TOKEN` matches OpenFGA preshared keys.
3. **OIDC failures** — verify discovery/JWKS URLs from inside the `serve` container (`wget` to IdP).
4. **Worker restarts** — ensure `serve` is healthy first; inspect AuthZ outbox dead letters via doctor.

## CI parity

Integration workflow runs `scripts/compose-smoke.sh` with `deploy/compose/docker-compose.test.yaml` (Dex OIDC). Use the same overlay locally:

```bash
COMPOSE_TEST_OVERLAY=deploy/compose/docker-compose.test.yaml make compose-starter-smoke
```

## Related

- [ADR 0016](../adr/0016-compose-first-production-release.md)
- [Release gates](./release-gates.md)
- [Backup and restore](./backup-restore.md)
- [Coolify / xsama promotion](../deploy/coolify/README.md)
