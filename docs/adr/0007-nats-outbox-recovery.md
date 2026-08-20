# ADR 0007: NATS Outbox and Recovery Authority

## Status

Accepted

## Context

JetStream is at-least-once. Claiming end-to-end exactly-once across PostgreSQL, S3, parsers, and indexes is false. Operators need a clear recovery authority.

## Decision

1. **PostgreSQL transactional outbox is the authoritative event source** for the starter profile.
2. Relay publishes with stable `Nats-Msg-Id=event_id`, durable pull consumers, explicit ack after local commit, inbox receipts `(consumer_name, event_id)`, bounded retries/DLQ, and reconciliation.
3. Never claim end-to-end exactly-once.
4. Recovery modes:
   - **Mode A (recommended):** restore PostgreSQL outbox + evidence; rebuild JetStream via relay
   - **Mode B:** JetStream snapshot + DB for faster catch-up (optional)
5. Do not delete outbox rows on publish acknowledgement alone; retain through a durable high-water mark and reconciliation window.
6. NATS subjects and derived events are **internal in v1**. Cross-platform consumers use gateway-owned cursor feeds or signed webhooks.

## Consequences

- Clear RPO story for backup/restore.
- No public second data plane via JetStream.