# ADR 0006: OpenFGA Policy Ownership and Consistency

## Status

Accepted

## Context

The reference doc listed OpenFGA, OPA, and Cedar with overlapping concerns, and advertised `at_least_as_fresh` consistency. OpenFGA today exposes `MINIMIZE_LATENCY` and `HIGHER_CONSISTENCY` only—no causal ZedToken equivalent.

## Decision

1. **OpenFGA is the only policy engine in v1** for relationship authorization.
2. Deterministic disclosure/redaction/purpose/classification obligations live in reviewed Go code behind `PolicyProvider`. Do not deploy OPA or Cedar in v1.
3. Consistency mapping:
   - `min_latency` → OpenFGA `MINIMIZE_LATENCY`
   - revocation-sensitive and `fully_consistent` → OpenFGA `HIGHER_CONSISTENCY` + no authz decision cache + successful tuple-write acknowledgement
4. Do **not** claim Zookie-style `at_least_as_fresh` until a provider with causal tokens is selected.
5. Pin authorization model IDs; shadow-test model rollouts before promotion.
6. OpenFGA is the default bundled adapter; external OpenFGA attach is supported via endpoint/store/model config.

## Consequences

- Honest revocation SLOs measurable against OpenFGA reality.
- Single decision orchestrator in the gateway; obligations never duplicate allow/deny.