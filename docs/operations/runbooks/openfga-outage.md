# Runbook: OpenFGA outage

## Symptoms

- AuthZ checks fail or return unavailable
- Alert `ContextFabricOpenFGAOutage`
- Worker logs: OpenFGA HTTP 5xx/timeout
- Search/graph may fail closed (deny or empty)

## Diagnosis

```bash
curl -s "$OPENFGA_API_URL/stores/$OPENFGA_STORE_ID" \
  -H "Authorization: Bearer $OPENFGA_API_TOKEN"
context-fabric doctor
```

Confirm store ID, model ID, and API token are pinned (not placeholders).

## Remediation

1. Restore OpenFGA service (Compose/K8s dependency health)
2. Validate network from worker → OpenFGA (private network only)
3. If model was recreated, re-run bootstrap and update `OPENFGA_MODEL_ID`
4. After recovery, worker drains outbox automatically; watch `outbox_pending`

## Fail-closed behavior

Context Fabric does **not** bypass AuthZ during outage. Expect elevated 403/503
until OpenFGA is healthy. Do not enable demo/memory auth in production.
