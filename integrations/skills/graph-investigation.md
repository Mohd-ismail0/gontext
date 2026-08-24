# Skill: graph-investigation

Non-authoritative guidance for subgraph exploration.

## When

Use `context.graph` when you have a seed `resource_id` and need neighbors (parent, assignee, related docs).

## How

1. Start with `depth: 1`, modest `max_nodes`, and optional `predicates`.
2. Read returned nodes/edges/citations only — AuthZ already filtered endpoints.
3. One optional deeper call (depth 2) if the first hop is insufficient.
4. Prefer `context.get` on a specific neighbor rather than unbounded traversal.

## Never

- Assume manager/org membership implies `restricted_reader` visibility.
- Follow edges across organizations without an explicit approved share (out of v1 normal path).
