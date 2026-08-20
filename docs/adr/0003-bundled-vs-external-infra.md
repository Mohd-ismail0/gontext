# ADR 0003: Bundled vs External Infrastructure

## Status

Accepted

## Context

The architecture promises deployer-neutral use of existing IdP, PostgreSQL, object storage, event bus, and authorization systems. Bundling everything forces forks for orgs with managed RDS/OpenFGA.

## Decision

Every dependency port exposes `connection_mode: bundled | external`:

- PostgreSQL (+ pgvector)
- S3-compatible evidence store
- NATS JetStream
- OpenFGA
- OIDC IdP

`context-fabric doctor` validates DNS, TLS, credentials, pgvector, RLS roles, JetStream persistence, S3 versioning/lock capabilities, OpenFGA store/model, OIDC issuer/audience, clock skew, and private-network exposure **before** traffic is accepted.

Minimum capability matrix is enforced at preflight; missing capabilities fail closed with actionable errors.

## Consequences

- Homelab gets one-command bundled demo.
- Enterprises attach existing infra without code changes.
- Capability gaps (e.g. no object lock) surface as blocked deletion manifests, not silent non-compliance.