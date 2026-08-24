# Skill: brief-assembly

Non-authoritative guidance for bounded briefs.

## When

Use `context.brief` when you already know one or more `resource_ids` and need a cited assembly for a stated `purpose`.

## How

1. Prefer brief over search when IDs are known (ADR 0015 step 1).
2. Pass `purpose` and honor `max_items`.
3. Present packet citations and redacted fields as returned — do not reconstruct omitted text.
4. If the packet is empty or denied, diagnose with decision tooling or `context.request_access`.

## Never

- Expand the brief by re-fetching evidence blobs or quarantined attachments.
- Treat brief content as write-back or send-capable actions (read-only MCP in v1).
