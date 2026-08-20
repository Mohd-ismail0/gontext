# ADR 0014: Durable AuthZ Tuple Synchronization

## Status

Accepted

## Context

Knowledge `parent` edges require matching OpenFGA `parent` tuples for ACL inheritance. Writing tuples after the ledger commit created split-brain: intake could succeed while AuthZ sync failed, and idempotent replay never re-synced.

## Decision

1. **Transactional outbox for AuthZ tuples.** When a knowledge `parent` edge with `sync_authz_parent=true` is written, enqueue an AuthZ tuple operation in the **same** ledger transaction (`authz_tuple_outbox`). Never call OpenFGA from the request path after commit.
2. **Worker applies tuples with lease + backoff.** A background worker claims pending rows, calls `RelationshipWriter.WriteTuples`, marks applied on success, and retries with exponential backoff on failure. Dead-letter after max attempts remains observable via ops/readiness.
3. **Reconciliation.** Periodically compare active parent edges requiring inheritance against applied outbox rows / OpenFGA and re-enqueue missing work.
4. **Idempotency.** Duplicate intake returns the committed result; pending AuthZ work continues independently. Tuple writes must tolerate already-exists duplicates.
5. **Deletes/tombstones.** Tombstoning a knowledge parent edge enqueues a delete tuple operation so inheritance is revoked.
6. **Fail-closed readiness.** Serve readiness fails if AuthZ outbox lag exceeds SLO or the OpenFGA writer is unavailable when parent sync is required.

## Consequences

- No accepted parent edge can permanently diverge from its required OpenFGA tuple without operator-visible lag.
- Memory and HTTP OpenFGA adapters both implement `RelationshipWriter`.
- Demo/memory profiles may apply tuples in-process via the same outbox APIs for parity.
