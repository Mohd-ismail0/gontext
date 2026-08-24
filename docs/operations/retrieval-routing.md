# Bounded adaptive retrieval routing (ADR 0015)

Planning stays in the **agent harness**, not an LLM inside Context Fabric.
The server AuthZ-gates every tool call and enforces absolute caps.

## Deterministic routing (max 3 retrieval rounds)

1. **Known resource ID** → `context.get` (or `context.brief` when a summary is enough).
2. **Relationship question with a seed** → `context.graph` with `depth=1`, `max_nodes≤50`.
3. **Unknown seed** → ranked `context.search` (Postgres FTS on thin projection).
4. **Insufficient evidence** → one query refinement **or** one depth-2 graph call (not both in the same round unless citations are empty).
5. **Stop** or call `context.request_access` after at most **three** retrieval rounds.

## Absolute server caps (not overridable by skills)

| Cap | Value |
|-----|-------|
| Graph depth | 0–4 (default 1) |
| Graph max_nodes | 1–200 (default 50; seed retained) |
| Search max_items | ≤50 |
| Dense/hybrid fusion | Disabled until held-out eval gate |

## Truncation

When `truncated=true`, `next_cursor` is a **signal only**. Resume pagination is not guaranteed in v1—re-query with adjusted `depth` / `max_nodes` / `predicates` / query text.

## Security non-negotiables

- Skills and prompts are **not** an authorization boundary.
- Tags/filters AND-narrow only; they never grant access.
- AuthZ `can_read` BatchCheck runs before ledger hydration.
- Graph edges require **both** endpoints to survive AuthZ + purpose policy.
