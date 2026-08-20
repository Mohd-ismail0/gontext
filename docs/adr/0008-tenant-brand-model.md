# ADR 0008: Tenant and Brand Model

## Status

Accepted

## Context

Prior docs mixed `tenant_id`, `logto_org_id`, and `brand_id`. Multi-brand deployments need a governed scope without letting tags grant access.

## Decision

1. Canonical tenancy key is `organization_id`.
2. Hierarchy: `organization → context_space → resource → record → derived projection`.
3. `brand` is an optional governed child scope with an opaque ID and OpenFGA relation when needed.
4. A display/search tag such as `brand:xsama` **never** establishes tenancy or visibility.
5. Path `{orgId}` is a consistency assertion that must match the validated token/org mapping; headers, prompts, filters, tags, and MCP args never choose a tenant.

## Consequences

- RLS policies key solely on `organization_id`.
- Brand partitioning is explicit in authz, not inferred from free tags.