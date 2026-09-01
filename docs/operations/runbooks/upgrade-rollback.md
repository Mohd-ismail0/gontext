# Runbook: upgrade and rollback

## Upgrade (Compose starter)

Production upgrades **pin image digest** (semver tags are pointers only). See [ADR 0016](../../adr/0016-compose-first-production-release.md).

### Pre-flight

1. Review release notes and migration head delta.
2. Take a fresh backup ([backup-restore.md](./backup-restore.md)).
3. Verify `make release-check` green on the target tag in CI.

### Upgrade steps

```bash
# Pull signed digest (example)
export TARGET_IMAGE=ghcr.io/mohd-ismail0/gontext@sha256:<digest>

bash scripts/upgrade.sh --image "$TARGET_IMAGE" --env-file deploy/compose/.env.starter
```

Or manually:

1. `docker pull` pinned digest
2. Set `CONTEXT_FABRIC_IMAGE` in `.env.starter`
3. `docker compose ... run --rm migrate`
4. Recreate `worker`, then `serve`
5. `doctor` + `compose-smoke`

### Post-upgrade verification

- `/health/ready` green
- `GET /v1/system/version` shows expected `product_version`
- AuthZ outbox dead count = 0
- Optional: `cf conformance run` against staging

## Rollback

Rollback is **restore previous digest**, not `docker compose down` alone.

1. **Stop traffic** at Caddy / LB.
2. **Revert image** — set `CONTEXT_FABRIC_IMAGE` to previous known-good digest in `.env.starter`.
3. **Schema** — if forward migration ran, restore Postgres from pre-upgrade backup **or** run down-migration only when release documents N-1 compatibility.
4. **Recreate** worker → serve.
5. **Doctor + smoke** — confirm ready and tombstone dominance.

### When to full restore instead of digest rollback

- Migration is not N-1 compatible
- Evidence or OpenFGA state diverged during failed upgrade
- AuthZ outbox dead letters persist after reconcile

Follow [backup-restore.md](./backup-restore.md) full restore path.

## Helm (scaled profile)

1. `helm upgrade` with pinned `image.digest` in values.
2. Wait for migrate Job completion.
3. Rolling restart worker Deployment, then serve.
4. Rollback: `helm rollback <release> <revision>` + verify migration compatibility.

## Related

- [deploy-starter-compose.md](../deploy-starter-compose.md)
- [upgrade-order.md](../upgrade-order.md)
