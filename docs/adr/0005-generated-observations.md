# ADR 0005: Generated Observations vs Agent Actions

## Status

Accepted

## Context

Organizations need a safe “memory write” primitive without granting agents arbitrary mutation or send powers.

## Decision

Separate **context contribution** from **agent action**:

- V1 permits registered sources and scoped services to ingest **source records** or **generated observations**.
- Generated observations are separate revisions with trust=`generated` or `user_claim`, lower authority, TTL/retention, review state, and **no overwrite** of source-authoritative facts.
- Agent write/send tools (`draft.reply`, outbound send, etc.) remain **out of v1** and require explicit scopes, budgets, and human approval when added.

## Consequences

- Agents can store cited observations without becoming a second source of truth.
- Prompt injection cannot escalate into irreversible external actions in v1.