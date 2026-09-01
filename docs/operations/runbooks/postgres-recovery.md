# Runbook: Postgres recovery

## Symptoms

- Serve/worker crash loops on DB connect
- Alert `ContextFabricPostgresDown` or `ContextFabricPostgresLatencyHigh`
- `doctor: postgres` failures

## Diagnosis

```bash
context-fabric doctor
psql "$POSTGRES_DSN" -c 'SELECT 1'
psql "$POSTGRES_ADMIN_DSN" -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 5"
```

Verify `context_gateway` role exists with `NOBYPASSRLS`.

## Remediation

1. Restore Postgres primary (vendor/operator procedure)
2. Confirm RLS policies intact on `records`, `legal_holds`, `outbox`, `search_documents`
3. Re-run grants: `context-fabric bootstrap` (admin DSN)
4. Rolling restart serve + worker after DB is healthy

## Data integrity checks

- Migration head matches embedded files (`doctor` migration head check)
- Tombstoned records still dominate search after recovery
- Legal holds table present (migration 009)

See [restore.md](./restore.md) for full point-in-time recovery.
