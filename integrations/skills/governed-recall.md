# Skill: governed-recall

Non-authoritative routing for sparse-first recall (ADR 0015).

## Routing

1. **Known `resource_id`** → `context.get` (or `context.brief` for multi-id assemble).
2. **Relational question with a seed** → `context.graph` (depth 1 default; hard max depth 4, max nodes 200).
3. **Unknown seed** → `context.search` (policy-first ranked lexical / FTS). Pass a clear `purpose`.
4. If evidence is thin, at most **one** refinement (narrower query or depth-2 graph). Server caps remain absolute.
5. Stop after **at most three** retrieval rounds, or call `context.request_access`.

## Caps

Respect tool descriptor caps (`max_items`, depth, nodes). If `truncated` is set, re-query with tighter bounds — do not invent resume protocols.

## Never

- Claim hybrid/dense fusion unless the deployment has enabled it behind evaluation gates.
- Hydrate or quote content the packet redacted or denied.
