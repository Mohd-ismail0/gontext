# Runbook: Backup failure

## Symptoms

- Alert `ContextFabricBackupFailure`
- Cron/host `scripts/backup.sh` non-zero exit
- Missing or stale backup artifacts on host/volume

## Diagnosis

```bash
bash scripts/backup.sh --dry-run   # if supported
journalctl -u context-fabric-backup --since "1 hour ago"
ls -la /var/backups/context-fabric/
```

Verify `POSTGRES_ADMIN_DSN`, S3 credentials, and disk space on backup host.

## Remediation

1. Fix underlying error (disk full, wrong DSN, MinIO unreachable)
2. Re-run backup manually: `context-fabric backup` or `bash scripts/backup.sh`
3. Validate backup manifest checksum and timestamp
4. Document incident; schedule restore drill if backup gap > RPO target

## RPO target

Production target: **≤30 minutes** data loss (see release gates P0-7).
Until soak metrics confirm RPO, treat any missed backup as P0.
