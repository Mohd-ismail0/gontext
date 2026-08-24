# ADR 0015: Sparse-First Retrieval and Portable MCP Distribution

## Status

Accepted

## Context

Context Fabric is a multi-tenant organizational context plane. Production profiles require real OIDC, OpenFGA, and Postgres. Agentic clients will consume the platform primarily via MCP. Industry evidence (adaptive GraphRAG, A2RAG, hybrid BM25+dense evaluations) shows that always-on dense retrieval and always-on deep graph traversal waste cost on easy queries while still missing precise identifiers. Skills and prompts must never act as an authorization boundary.

## Decision

1. **Production-first.** Ship a governed server that is correct and measurable on `starter`/`xsama`/`scaled` before polishing optional harness skill packs.
2. **Generic MCP distribution.** MCP (`POST /mcp` + RFC 9728 protected-resource metadata) is the portable interface. Optional skill/reference packs teach orchestration only; they do not grant access, widen scopes, or choose tenants.
3. **Sparse-and-graph-first retrieval.**
   - Known resource ID → `context.get` / `context.brief`.
   - Relational question with a seed → `context.graph` (default depth 1, hard max depth 4, max nodes 200).
   - Unknown seed → ranked PostgreSQL FTS on a thin projection (IDs, title, kind, labels, short summary).
   - Insufficient evidence → at most one query refinement or depth-2 graph call (harness-enforced; server caps remain absolute).
   - Stop or `context.request_access` after at most three retrieval rounds.
4. **Dense retrieval is gated.** Embeddings are optional, org-opt-in, hash-deduplicated, async, and enabled only after held-out evaluation shows material Recall@K / nDCG gains within latency/cost SLOs. Never vectorize evidence blobs or placeholders by default.
5. **AuthZ-before-hydrate remains absolute.** Candidates are org/filter-scoped; OpenFGA `can_read` BatchCheck runs before ledger hydration; edges require both endpoints to survive AuthZ and policy (ADR 0013).
6. **Graph cursor.** `next_cursor` may be emitted for truncation signaling. Resume pagination is not required in v1; clients re-query with adjusted depth/`max_nodes`/predicates. OpenAPI must not imply a working resume protocol until implemented.
7. **Out of v1 (unchanged from ADR 0012):** customer-facing auth, agent write/send, attachment parsing beyond quarantine, third-party plugins, public NATS, OPA/Cedar/SpiceDB/Kafka/OpenSearch/Qdrant/Temporal, SCIM as live authz, analytical GraphRAG engines as system of record.

## Consequences

- Search marketing language must say "policy-first ranked lexical search" (not "hybrid") until dense fusion ships behind a feature flag.
- MCP tool descriptors must advertise caps, purpose, and truncation semantics accurately.
- Conformance and CI must eventually exercise Postgres + OpenFGA + OIDC, not memory-only demos.
- Skills packages version independently and are labeled non-authoritative.
