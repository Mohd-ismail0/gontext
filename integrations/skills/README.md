# Context Fabric skill packs (non-authoritative)

These markdown skills teach **orchestration only**: which MCP tools to call,
in what order, and when to stop or request access.

They are **not** an authorization boundary.

- Skills never grant, deny, or widen access.
- Skills never choose a tenant or override `organization_id`.
- Skills never bypass OpenFGA / policy checks performed by the server.
- AuthZ remains server-side (ReBAC + purpose/delegation policy) on every
  `tools/call` and REST retrieval path (see ADR 0015).

Use these packs with a governed Context Fabric deployment. Prefer the live
MCP tool descriptors and OpenAPI contract over skill text when they disagree.
