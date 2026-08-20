# Backup and restore

Context Fabric treats PostgreSQL as the authoritative ledger, S3-compatible object storage as versioned evidence, and OpenFGA as relationship state. Backups must be coordinated so a restore can replay tombstones before query traffic resumes.

## What to capture

| Component | Artifact | Notes |
|-----------|----------|-------|
| PostgreSQL | logical dump (`pg_dump`) | Include all schemas; prefer consistent snapshot |
| Evidence (S3/MinIO) | bucket sync | Versioned objects; preserve delete markers |
| OpenFGA | store/model/tuple export | Placeholder until operator tooling lands |
| Config | profile + secret *references* | Never dump live secrets into the bundle |

## Backup procedure

1. Quiesce or snapshot writers when possible (`worker` drain).
2. Run [`scripts/backup.sh`](../../scripts/backup.sh) with `POSTGRES_DSN`, `S3_BUCKET`, and an output directory.
3. Record product version, authz model ID, and migration head in the bundle manifest.
4. Store the bundle offline with encryption at rest.

## Restore procedure

1. Provision empty Postgres / bucket / OpenFGA store.
2. Run [`scripts/restore.sh`](../../scripts/restore.sh).
3. Apply migrations if the dump is schema-light.
4. **Replay tombstones / revocations before opening serve ready.**
5. Run `context-fabric doctor` and a conformance search that proves deleted resources remain unretrievable.

## Verification

- `GET /health/ready` succeeds.
- Export a support bundle (`cf ops support-bundle`) without secrets.
- Spot-check one tombstoned resource via search/get returns empty citations.
