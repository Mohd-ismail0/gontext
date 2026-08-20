# ADR 0011: Connector Trust Tiers

## Status

Accepted

## Context

OCI isolation is correct for untrusted plugins but heavy for a single first-party Chatwoot connector on day one.

## Decision

| Tier | Examples | Execution | DB access |
|------|----------|-----------|-----------|
| Core | serve, worker, ledger, OpenFGA adapter | reviewed core process | Limited, service-specific |
| First-party connector (Tier 0) | Chatwoot in v1 | `connector` role using public intake APIs; may start as digest-pinned OCI with no DB creds | No |
| Third-party plugin (Tier 1) | customer source/parser | signed OCI/WASM with default-deny egress | No |
| Deterministic transform | redaction/tag transform | WASM with explicit host capabilities | No |

Chatwoot v1 uses Tier 0/1 boundary: separately authenticated `connector chatwoot` with **no** PostgreSQL, NATS, OpenFGA, or broad evidence credentials—only source registration, MappingSpec, evidence upload, and generic intake.

## Consequences

- Same intake contract for all connectors.
- Containment tests apply before third-party plugins.