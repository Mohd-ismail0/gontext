# Capacity SLO baselines

Capacity and latency SLOs for Context Fabric are **measurement-derived**, not guessed
from architecture diagrams. Numbers below stay placeholders until a staging soak
produces stable percentiles under representative tenant mix and load.

## Principle

1. Instrument the metric in staging and production.
2. Soak long enough to cover peak and quiet windows.
3. Set the SLO from observed p95/p99 (plus safety margin), then wire alert burn rates.
4. Revisit after material infra or schema changes.

## Metrics to collect

| Metric | Definition | Staging baseline | Production target |
|--------|------------|------------------|-------------------|
| Search p95 latency | End-to-end `context.search` / MCP search latency (authn → AuthZ → index → packet) | TBD after staging soak | TBD after staging soak |
| Revocation visibility | Time from principal/delegation revoke until sensitive retrieval returns deny/empty under `fully_consistent` | TBD after staging soak | TBD after staging soak |
| Deletion time-to-zero | Time from delete accept until search/get/brief return no citations for that resource (all derivatives) | TBD after staging soak | TBD after staging soak |
| Restore RPO | Maximum acceptable ledger/evidence data loss on restore drill | TBD after staging soak | TBD after staging soak |
| Restore RTO | Time from restore start until ready + tombstone replay verified | TBD after staging soak | TBD after staging soak |
| Onboarding duration | Time for a new org from bootstrap to first successful authorized search | TBD after staging soak | TBD after staging soak |

## Related gates

Release admission still requires the qualitative gates in [`release-gates.md`](./release-gates.md)
(authz matrix, tombstone dominance, export integrity). Numeric capacity gates land only after
the staging soak fills the table above.
