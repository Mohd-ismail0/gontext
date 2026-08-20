# ADR 0010: Change Feed and Webhook Exposure

## Status

Accepted

## Context

Downstream automation (CRM sync, analytics, agents) need lifecycle notifications. Exposing NATS derived streams invites shadow context stores.

## Decision

1. Public change delivery is **gateway-owned** only:
   - Organization-scoped **cursor feed** (pull)
   - Signed **webhooks** (push)
2. Public change events contain opaque IDs, revision/state, schema/profile versions, and cursor—**never** content, vectors, prompts, or authorization tuples.
3. Consumers hydrate via authorized `search`/`get`/`brief`.
4. Semantics: stable event IDs, monotonic org-scoped cursors (no global order), HMAC or asymmetric signatures, replay window, exponential retry, DLQ/inspection, endpoint verification, secret rotation, pull catch-up.
5. Internal NATS channels remain implementation detail in v1.

## Consequences

- Cross-platform integration without a second data plane.
- Webhook receivers stay idempotent on `event_id`.