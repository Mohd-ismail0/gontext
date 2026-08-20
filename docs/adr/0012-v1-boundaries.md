# ADR 0012: V1 Product Boundaries

## Status

Accepted

## Context

Scope creep (attachments, customer callers, agent send, Kafka, OPA) threatens delivery of a portable governed context plane.

## Decision

**In v1:**

- Employees, service principals, delegated agents
- Generic CloudEvents intake + MappingSpec
- Chatwoot text connector + synthetic second connector for conformance
- REST + read-only MCP retrieval (`search`, `get`, `brief`, `graph`, `request_access`); context is a ReBAC-gated knowledge graph (ADR 0013)
- Cursor feed + signed webhooks (metadata only)
- Organization export/import
- OpenFGA + Go PolicyProvider
- PostgreSQL/RLS + pgvector, S3 evidence, NATS JetStream
- Compose / Coolify / Helm from one image
- `cf` CLI bootstrap/doctor/diagnose/ops

**Out of v1 (later gates):**

- Customer-facing REST/MCP authentication
- Agent write/send actions
- Attachment parsing (Docling/Tika/ClamAV) beyond quarantine
- Third-party plugins
- Public internal NATS streams
- OPA/Cedar, SpiceDB, Kafka, OpenSearch, Qdrant, Temporal
- SCIM as live authz

## Consequences

- Honest “v1 complete” definition tied to onboarding, portability, and security gates.