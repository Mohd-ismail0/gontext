# Context Fabric

Self-hosted, multi-tenant **organizational context plane** for governed intake, provenance, and employee/agent retrieval over REST and MCP. One signed multi-architecture Go image; packaging profiles do not fork authorization or canonical semantics.

## Quickstart (Compose)

```bash
cp deploy/compose/.env.example deploy/compose/.env
# edit secrets in deploy/compose/.env

make docker-build
make compose-up
# or minimal one-process demo:
docker compose -f deploy/compose/docker-compose.minimal.yaml --env-file deploy/compose/.env up --build
```

Public surface: **serve only** on port `8080` (`/health/live`, `/health/startup`, `/health/ready`, `/v1/system/version`). Postgres, MinIO, NATS, and OpenFGA stay on the private Compose network.

Useful targets: `make build`, `make test`, `make lint`, `make migrate`, `make doctor`.

## Image roles

| Role | Purpose |
|------|---------|
| `serve` | REST, MCP, authn/authz, retrieval, packets, audit, change feed |
| `worker` | Outbox relay, projections, deletion/export jobs |
| `connector` | Isolated source connectors (no DB/NATS/OpenFGA creds) |
| `migrate` / `bootstrap` | Forward migrations; one-time seeding |
| `doctor` | Preflight capability and connectivity checks |
| `backup` / `restore` / `reconcile` | Operational recovery |
| `all` | Demo-only serve+worker in one process |

```bash
docker run --rm ghcr.io/xsama/context-fabric:1.0.0 serve
docker run --rm ghcr.io/xsama/context-fabric:1.0.0 worker
```

## Profiles

Concrete configs under [`deploy/profiles/`](deploy/profiles/) (`demo`, `starter`, `xsama`, `scaled`) validate against [`deploy/schema/config.schema.json`](deploy/schema/config.schema.json). Secrets are classified (never valued) in [`deploy/schema/secrets.schema.json`](deploy/schema/secrets.schema.json).

| Profile | Packaging |
|---------|-----------|
| `demo` | Bundled deps, role `all` |
| `starter` | Compose bundled PG/NATS/MinIO/OpenFGA; external OIDC |
| `xsama` | Coolify + private LXCs; public serve only — see [`deploy/coolify/README.md`](deploy/coolify/README.md) |
| `scaled` | Helm, external deps, independently scalable serve/worker |

Helm chart: [`deploy/helm/context-fabric/`](deploy/helm/context-fabric/).

## Architecture pointers

- ADRs: [`docs/adr/`](docs/adr/) — especially [0001 one-image roles](docs/adr/0001-one-image-role-topology.md), [0002 profiles](docs/adr/0002-deployment-profiles.md), [0003 bundled vs external](docs/adr/0003-bundled-vs-external-infra.md), [0012 v1 boundaries](docs/adr/0012-v1-boundaries.md)
- Threat model: [`docs/threat-model/`](docs/threat-model/)
- Design reference: [`context-fabric-stress-test.md`](context-fabric-stress-test.md)

## V1 boundaries

**In v1:** employees/service principals/delegated agents; generic CloudEvents intake + MappingSpec; Chatwoot text connector + synthetic conformance connector; REST + read-only MCP (`search`, `get`, `brief`, `request_access`); cursor feed + signed webhooks (metadata only); org export/import; OpenFGA + Go `PolicyProvider`; PostgreSQL/RLS + pgvector, S3 evidence, NATS JetStream; Compose / Coolify / Helm from one image; `cf` CLI.

**Out of v1:** customer-facing REST/MCP auth; agent write/send; attachment parsing beyond quarantine; third-party plugins; public NATS streams; OPA/Cedar/SpiceDB/Kafka/OpenSearch/Qdrant/Temporal; SCIM as live authz.

See [ADR 0012](docs/adr/0012-v1-boundaries.md).
