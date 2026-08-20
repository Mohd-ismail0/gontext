# ADR 0013: Knowledge-Graph-First Context Plane

## Status

Accepted (hardened)

## Context

Organizational context is inherently relational. Callers must see only the subgraph their access profile permits. Knowledge edges must never become the ACL system.

## Decision

1. **Every canonical `Record` is a graph node.** Kinds (`document`, `case`, `person`, `observation`, `PLACEHOLDER`, …) are node types, not separate stores.
2. **Knowledge edges are first-class ledger objects** (`graph_edges`): directed, typed predicates (`parent`, `related_to`, `mentions`, `derived_from`, `assigned_to`, …), org-scoped, tombstoneable. Edges are facts, not grants.
3. **Access profile = OpenFGA ReBAC + Go PolicyProvider obligations.** A principal’s visible universe is the set of nodes that independently pass org isolation, OpenFGA `can_read`, policy/purpose/classification, and lifecycle checks.
4. **Edge visibility rule:** an edge is returned only when **both** endpoints are in the final visible-node set after AuthZ **and** policy. No dangling endpoints, no existence leaks via counts/titles/topology.
5. **Traversal is AuthZ-first then policy-final:** candidates → bounded expansion → `BatchCheck` → hydrate → policy/lifecycle filter → edge filter on the surviving node set. Never hydrate then filter; never trust client-supplied edge sets.
6. **OpenFGA `parent` is the ACL inheritance edge.** Knowledge `parent` edges that request inheritance enqueue durable AuthZ tuple outbox work (ADR 0014). `related_to` / `mentions` never sync AuthZ.
7. **`visibility_ref` is source metadata, not an ACL grant.** Only explicit `parent_resource_id` or reviewed MappingSpec parent rules may request AuthZ inheritance.
8. **`PLACEHOLDER` nodes** are created for missing edge endpoints: non-indexable, non-searchable, non-retrievable, no default reader grant. Authoritative intake promotes them atomically to real records.
9. **All read surfaces are graph queries.** `search`, `get`, `brief`, and `graph` return a `ContextPacket` with `nodes`, `edges`, citations, and truncation metadata.
10. **External temporal graph engines stay optional projections** rebuilt from the ledger; they are not truth or authorization.

## Consequences

- Production OpenFGA and memory AuthZ must enforce the same pinned model.
- Export/import, backup/restore, and conformance must include graph edges and AuthZ sync state.
- Tests must prove denied/placeholder neighbors never leak.
