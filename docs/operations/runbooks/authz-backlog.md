# Runbook: AuthZ outbox backlog

## Symptoms

- `context_fabric_outbox_pending` gauge rising
- Alert `ContextFabricAuthzBacklog` or `ContextFabricAuthzBacklogCritical`
- Revokes/deletes slow to propagate to OpenFGA

## Diagnosis

```bash
curl -s localhost:9090/metrics | grep outbox_pending
curl -s -H "Authorization: Bearer $TOKEN" \
  localhost:8080/v1/organizations/$ORG/ops/lag | jq .
```

Inspect worker logs for OpenFGA write errors and lease contention.

## Remediation

1. Confirm worker pod/process is running (`CONTEXT_FABRIC_ROLE=worker`)
2. Verify OpenFGA reachable: `context-fabric doctor`
3. Check for dead-letter tuples in ready detail; replay after fixing root cause
4. Temporarily scale worker replicas (Helm `worker.replicas`)

## Prevention

- Pin `OPENFGA_MODEL_ID` after bootstrap
- Avoid bulk edge writes without worker capacity
- Monitor `context_fabric_authz_batch_checks` vs outbox drain rate
