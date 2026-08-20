# Diagnostics

## Quick checks

```bash
curl -sS "$CONTEXT_FABRIC_URL/health/live"
curl -sS "$CONTEXT_FABRIC_URL/health/ready"
curl -sS "$CONTEXT_FABRIC_URL/v1/system/version"
cf doctor
```

## Decision reconstruction

Privileged operators can reconstruct an allow/deny without plaintext content:

```bash
cf diagnose decision --org "$ORG" --audit-id "$AUDIT_ID"
```

This uses `GET /v1/organizations/{orgId}/context/diagnose/decision/{auditId}` and returns authz/policy revisions plus the sanitized audit row.

## Lag and support bundles

```bash
cf ops lag --org "$ORG"
cf ops support-bundle --org "$ORG"
```

Bundles include version/health metadata only — no tokens, queries, or evidence bytes.

## Common signals

| Symptom | Likely cause | Next step |
|---------|--------------|-----------|
| ready=false | migrations / authz model | check migrate logs, model pin |
| empty search for known resource | tombstone, purpose, or AuthZ deny | diagnose decision; inspect state |
| webhook duplicates | receiver not idempotent on `event_id` | replay + catch-up via cursor feed |
| export hash fail | tampered or partial manifest | re-export; never skip verification |
