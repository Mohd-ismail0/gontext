# Runbook: backup and restore

Host-managed backup per [ADR 0016](../../adr/0016-compose-first-production-release.md): the application image does not embed `pg_dump` or object-store clients. Run [`scripts/backup.sh`](../../../scripts/backup.sh) and [`scripts/restore.sh`](../../../scripts/restore.sh) from an ops host (or utility container) with tooling installed.

## RPO / RTO targets (starter profile)

| Metric | Target |
|--------|--------|
| RPO | ≤ 30 minutes |
| RTO | ≤ 60 minutes |

## Backup procedure

1. **Quiesce writers** (optional but recommended): scale worker to 0 or pause connectors.
2. **Export env** — load `deploy/compose/.env.starter` (or production secret store equivalents):
   - `POSTGRES_ADMIN_DSN`
   - `S3_BUCKET_RAW`, `S3_BUCKET_DERIVED`, `S3_BUCKET_QUARANTINE`
   - `OPENFGA_API_URL`, `OPENFGA_STORE_ID`, `OPENFGA_API_TOKEN`
3. **Run backup**:
   ```bash
   make backup BACKUP_DIR=/var/backups/context-fabric/$(date -u +%Y%m%dT%H%M%SZ)
   ```
4. **Verify manifest** — `manifest.json` includes `product_version`, `migration_head`, and bucket list.
5. **Store offline** — encrypt at rest; restrict access to platform + security roles.

### What is captured

| Component | Artifact |
|-----------|----------|
| PostgreSQL | `postgres.dump` (custom format) |
| Evidence | `evidence/<bucket>/` per bucket + version metadata |
| OpenFGA | `openfga/model.json`, `openfga/tuples.jsonl` (when `fga` CLI available) |
| Config refs | Profile + `.env.starter.example` copies (no live secrets) |

## Restore procedure

1. Provision empty Postgres, MinIO buckets (versioning enabled), and OpenFGA store.
2. **Stop serve/worker** — only gateway or maintenance page public.
3. **Restore**:
   ```bash
   export POSTGRES_ADMIN_DSN=...
   export S3_BUCKET_RAW=... S3_BUCKET_DERIVED=... S3_BUCKET_QUARANTINE=...
   make restore BACKUP_DIR=/var/backups/context-fabric/<timestamp>
   ```
4. **OpenFGA** — import model/tuples from bundle before traffic.
5. **Reconcile** — `scripts/reconcile.sh` (included in restore script).
6. **Doctor + smoke** — `context-fabric doctor`, `make compose-starter-smoke`.
7. **Tombstone dominance** — spot-check deleted resources remain unretrievable via search.

## Quarterly drill

- Restore to an isolated host using the same compose overlay.
- Record actual RTO; update [slo-baselines.md](../slo-baselines.md) when measured.

## Related

- [backup-restore.md](../backup-restore.md) (overview)
- [upgrade-rollback.md](./upgrade-rollback.md)
