# Skill: connect-auth

Non-authoritative orchestration for authenticating to Context Fabric MCP.

## Steps

1. Obtain a short-lived bearer token from the org's OIDC / agent credential flow (never embed secrets in prompts).
2. Confirm `GET /.well-known/oauth-protected-resource` (RFC 9728) matches the MCP resource URL.
3. `POST /mcp` with `initialize`, then `tools/list` — verify `context.search`, `context.get`, `context.brief`, `context.graph`, `context.request_access`.
4. Always pass `organization_id` (or `X-Organization-Id`) that the token is authorized for.

## Never

- Treat this skill as permission to access data.
- Invent scopes or forge local tokens in production.
