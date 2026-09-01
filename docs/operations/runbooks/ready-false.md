# Runbook: `/health/ready` returns false

## Symptoms

- Load balancer / Caddy marks upstream unhealthy
- `curl https://<host>/health/ready` returns 503 or `{"ready":false}`
- `cf doctor` reports dependency failures

## Immediate checks

1. **Readiness JSON** — `curl -s https://<host>/health/ready | jq .` for per-check detail (migrations, OpenFGA model pin, AuthZ outbox dead letters).
2. **Logs** — `docker compose logs serve worker --tail=200` (starter overlay) or Kubernetes pod logs.
3. **Doctor** — `make compose-starter-preflight` or `context-fabric doctor` from an ops host with env loaded.

## Common causes

| Check | Likely cause | Fix |
|-------|--------------|-----|
| `migrations` | Pending migration or failed migrate job | Run `migrate`; verify `schema_migrations` head matches image |
| `openfga_model` | Model ID mismatch or store unreachable | Re-run `bootstrap`; verify `OPENFGA_API_TOKEN` matches OpenFGA preshared key |
| `authz_outbox` | Dead-letter tuples | Run `scripts/reconcile.sh`; inspect `authz_tuple_outbox` for `dead` rows |
| `relationship_writer` | OpenFGA write path down | Check OpenFGA health; token and store ID |
| `oidc` | IdP discovery/JWKS unreachable | Verify `OIDC_ISSUER`, firewall, TLS trust from serve container |

## Escalation

- Do **not** enable `CONTEXT_FABRIC_ALLOW_*` escape hatches in production.
- If data plane is inconsistent after dependency outage, follow [upgrade-rollback.md](./upgrade-rollback.md) before forcing ready.

## Related

- [deploy-starter-compose.md](../deploy-starter-compose.md)
- [release-gates.md](../release-gates.md) P0 gates
