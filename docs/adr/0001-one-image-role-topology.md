# ADR 0001: One Image, Role Topology

## Status

Accepted

## Context

The reference architecture listed five deployable services (`gateway`, `ingester`, `projector`, `retriever`, `plugin-runner`, `mcp-adapter`). That packaging cost makes Compose/Coolify/Helm installs hard and creates second-data-plane risk if retrieval is split from authorization.

## Decision

Ship **one signed multi-architecture `context-fabric` image** with role subcommands:

| Role | Responsibility |
|------|----------------|
| `serve` | REST, MCP, authn, authz orchestration, retrieval, hydration, redaction, packets, audit, change feed |
| `worker` | Outbox relay, projection consumers, deletion/export jobs |
| `connector` | Isolated source connectors (e.g. Chatwoot); no DB/NATS/OpenFGA credentials |
| `migrate` | Forward schema migrations only |
| `bootstrap` | One-time org/model/role seeding |
| `doctor` | Preflight connectivity and capability checks |
| `backup` / `restore` / `reconcile` | Operational recovery |

Do **not** deploy a separate retriever or MCP data service in v1. MCP is an in-process surface on `serve` over the same application service as REST.

## Consequences

- One SBOM, one digest, one release cadence across Compose/Coolify/Helm.
- Module boundaries enforced in code review, not process isolation (except connectors).
- Scaled profile may run multiple replicas of `serve`/`worker` roles; never split retrieval from auth.