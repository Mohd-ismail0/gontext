# Runbook: Restore

## When to use

- Postgres data corruption or accidental deletion
- Evidence bucket loss (with ledger intact)
- Disaster recovery drill

## Preconditions

- Valid backup from `scripts/backup.sh` (ledger + optional evidence manifest)
- Maintenance window; scale serve to 0 or enable read-only if possible
- Admin DSN (`POSTGRES_ADMIN_DSN`) available

## Procedure

1. Stop serve and worker processes
2. Restore Postgres from latest backup:
   ```bash
   bash scripts/restore.sh /var/backups/context-fabric/LATEST
   ```
3. Run migration head check: `context-fabric doctor`
4. Verify tombstones replay: run deletion conformance / search drill
5. Restore S3 evidence buckets if included in backup manifest
6. Start worker first, then serve; confirm `/health/ready`

## Validation

- `schema_migrations` at head
- Sample authorized search returns expected citations
- Deleted resources remain absent (tombstone dominance)
- Legal holds block purge (`legal_holds` table)

## RTO target

Production target: **≤60 minutes** to ready (release gates P0-7).
