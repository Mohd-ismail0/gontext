# ADR 0013: Knowledge-Graph-First Context Plane

## Status

Accepted

## Context

Retrieval today returns flat citation lists. Organizational context is inherently relational (case → notes → messages → people → policies). Operators and agents need a single mental model: the plane is a knowledge graph, and each caller only sees the subgraph their access profile permits.

OpenFGA already models resource `parent` inheritance for `can_read` / `can_manage`. That is authorization topology, not the knowledge graph. Derived “graph facts” must never become the ACL system (ADR 0005, stress-test Graphiti guidance).

## Decision

1. **Every canonical `Record` is a graph node.** Kinds (`document`, `case`, `person`, `observation`, …) are node types, not separate stores.
2. **Knowledge edges are first-class ledger objects** (`graph_edges`): directed, typed predicates (`parent`, `related_to`, `mentions`, `derived_from`, `assigned_to`, …), org-scoped, tombstoneable. Edges are evidence of relationship, not grants.
3. **Access profile = OpenFGA ReBAC + Go PolicyProvider obligations.** A principal’s visible universe is the set of nodes for which `can_read` (or stronger) allows, after purpose/classification policy.
4. **Edge visibility rule:** an edge is returned only when **both** endpoints are independently allowed for the caller. Knowing that A→B exists must not leak the existence of a denied node.
5. **Traversal is AuthZ-first:** resolve candidates → expand edge frontier with hard caps (`depth`, `max_nodes`) → `BatchCheck` every node → hydrate → apply policy. Never hydrate then filter; never trust client-supplied edge sets.
6. **OpenFGA `parent` remains the ACL inheritance edge.** When a knowledge `parent` edge is written, operators/connectors SHOULD also write the matching OpenFGA `parent` tuple so inheritance stays consistent. Knowledge `related_to` / `mentions` never imply AuthZ.
7. **All read surfaces are graph queries.** `search`, `get`, `brief`, and `graph` return a `ContextPacket` that includes `nodes` and `edges` (plus citations for retrieval UX). Search is “ranked entry points into your visible subgraph.”
8. **External temporal graph engines (e.g. Graphiti) stay optional projections** rebuilt from the ledger; they are not the source of truth or of authorization.

## Consequences

- Product language and APIs treat context as a governed subgraph, not a document search box.
- Connectors and MappingSpec gain an explicit path to emit edges alongside records.
- MCP/REST grow a neighborhood/`graph` tool without forking AuthZ semantics.
- Tests must prove denied neighbors are omitted (no stubs that leak titles/IDs) and that tags/labels never widen node access.
