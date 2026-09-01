# Capacity SLO baselines

Capacity and latency SLOs for Context Fabric are **measurement-derived**, not guessed
from architecture diagrams. Numbers below stay placeholders until a staging soak
produces stable percentiles under representative tenant mix and load.

**Numbers remain TBD until a 72h soak** completes on a representative staging
profile (`starter` / `xsama` / `scaled`). Do not invent p95/p99 targets from
architecture diagrams alone.

## Principle

1. Instrument the metric in staging and production.
2. Soak long enough to cover peak and quiet windows (minimum **72 hours**).
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

## 72h soak worksheet (fill after staging run)

Record observations once per 4h window during a continuous 72h load test on the
target profile. Leave cells as `TBD` until the soak completes.

| Window (UTC) | Search p95 (ms) | Graph p95 (ms) | AuthZ batch p95 (ms) | Outbox max pending | Webhook success % | Notes |
|--------------|-----------------|----------------|----------------------|--------------------|-------------------|-------|
| 0–4h         | TBD             | TBD            | TBD                  | TBD                | TBD               |       |
| 4–8h         | TBD             | TBD            | TBD                  | TBD                | TBD               |       |
| 8–12h        | TBD             | TBD            | TBD                  | TBD                | TBD               |       |
| 12–24h       | TBD             | TBD            | TBD                  | TBD                | TBD               |       |
| 24–48h       | TBD             | TBD            | TBD                  | TBD                | TBD               |       |
| 48–72h       | TBD             | TBD            | TBD                  | TBD                | TBD               |       |

### Soak summary (post-run)

| Aggregate | Staging value | Proposed production SLO | Alert threshold |
|-----------|---------------|-------------------------|-----------------|
| Search p95 | TBD | TBD | TBD |
| Graph p95 | TBD | TBD | TBD |
| Revocation visibility p99 | TBD | TBD | TBD |
| Deletion time-to-zero p99 | TBD | TBD | TBD |
| Outbox pending steady-state max | TBD | TBD | TBD |
| Restore RPO (drill) | TBD | ≤30m | n/a |
| Restore RTO (drill) | TBD | ≤60m | n/a |

## Measurement plan

Until the 72h soak fills the table above, collect and trend these signals
(ADR 0015 / ops plan). Scrape `GET /metrics` where applicable:

| Signal | Source | Why |
|--------|--------|-----|
| `context_fabric_search_requests` | `/metrics` | Volume for search latency SLOs |
| `context_fabric_authz_batch_checks` | `/metrics` | AuthZ BatchCheck cost vs search/graph |
| `context_fabric_graph_requests` | `/metrics` | Graph hop load vs search |
| `context_fabric_outbox_pending` | `/metrics` | AuthZ outbox backlog / lag |
| `context_fabric_webhook_deliveries` | `/metrics` | Change-notification delivery health |
| `context_fabric_http_requests_total{route_class,status_class}` | `/metrics` | Request volume and status mix |
| `context_fabric_http_errors_total{route_class,error_class}` | `/metrics` | Client/server error rates |
| `context_fabric_dependency_latency_ms{dependency}` | `/metrics` | Last observed dependency RTT |
| `context_fabric_build_info` | `/metrics` | Deployed version/commit verification |
| Search / graph / brief latency histograms | app logs or future histogram metrics | p95/p99 for capacity SLOs |
| Revocation visibility samples | staged revoke drills | consistency mode correctness |
| Deletion time-to-zero samples | delete → retrieval drills | tombstone dominance |
| Restore RPO/RTO | backup/restore drills (`scripts/*.sh`) | DR gates |
| Onboarding duration | bootstrap → first authorized search | tenant readiness |

Wire alert burn rates only after the soak produces real percentiles — leave
staging/production cells as TBD until then.

## Related gates

Release admission still requires the qualitative gates in [`release-gates.md`](./release-gates.md)
(authz matrix, tombstone dominance, export integrity). Numeric capacity gates land only after
the staging soak fills the table above.
