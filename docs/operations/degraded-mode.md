# Degraded mode

When dependencies fail, Context Fabric prefers **fail closed** on disclosure over serving stale governed context.

## Modes

| Mode | Behavior |
|------|----------|
| Ready | Normal serve + worker |
| Degraded search | AuthZ/policy still required; may return empty packets with audit reason |
| Intake paused | Quota or outbox backpressure; clients see retryable errors |
| Authz unavailable | Deny new retrieval; do not fall back to open filters |
| Evidence unavailable | Citations may omit snippets; ledger metadata still authoritative |

## Operator actions

1. Mark ready false if AuthZ or ledger is unsafe (`GET /health/ready`).
2. Keep deletion visibility revocation online even if projection cleanup lags.
3. Prefer cursor feed catch-up after webhook outages; do not widen event payloads.
4. Exit degraded mode only after doctor checks and a tombstone dominance smoke test.
