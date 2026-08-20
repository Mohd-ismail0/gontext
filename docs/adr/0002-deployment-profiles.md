# ADR 0002: Deployment Profiles

## Status

Accepted

## Context

Organizations need demo, homelab, XSAMA, and Kubernetes paths without forking authorization or canonical semantics.

## Decision

Profiles choose **packaging only**:

| Profile | Image roles | Dependencies | Public surface |
|---------|-------------|--------------|----------------|
| `demo` | single process `all` (serve+worker) | bundled PG/NATS/MinIO/OpenFGA/test OIDC | serve only |
| `starter` / `xsama` | serve, worker, connector | bundled or external | serve only |
| `scaled` | independently scalable roles | external PG/S3/NATS/OpenFGA | serve only |

A versioned `DeploymentProfile` JSON Schema under `deploy/schema/` defines roles, `bundled|external` modes, minimum versions, secrets, persistence, network exposure, migration compatibility, and backup ownership.

Profiles **must not** change authorization, lifecycle, or canonical record semantics.

## Consequences

- Compose, Coolify, and Helm consume the same profile schema.
- Kafka/OpenSearch/Qdrant/Temporal remain measurement-gated adapters, not install-time choices in v1.