# ADR 0009: Action Naming Conventions

## Status

Accepted

## Context

OAuth scopes, delegation actions, MCP tools, and REST paths used inconsistent separators (`context:search` vs `context.search`).

## Decision

| Layer | Convention | Examples |
|-------|------------|----------|
| OAuth scopes | `context:*` | `context:search`, `context:read`, `context:ingest`, `context:audit_read`, `context:request_access`, `context:manage_policy` |
| Internal authz actions | `context.*` | `context.search`, `context.get`, `context.brief`, `context.request_access` |
| REST routes | resource-oriented | `POST .../context:search`, `GET .../resources/{id}`, `POST .../context:brief` |
| MCP tools | map 1:1 to application ops | `context.search`, `context.get`, `context.brief`, `context.request_access` |

A canonical **brief** REST operation exists so MCP does not invent an unmatched path.

## Consequences

- Single ApplicationService interface backs REST and MCP.
- Scope ceilings are coarse; resource visibility remains OpenFGA’s job.