# Context Fabric

Self-hosted, multi-tenant **organizational context plane** for governed intake, provenance, and employee/agent retrieval over REST and MCP. One signed multi-architecture Go image; packaging profiles do not fork authorization or canonical semantics.

## Quickstart (demo / memory)

Fastest path for **development** — no Postgres, OpenFGA, or Compose required:

```bash
make build
CONTEXT_FABRIC_MEMORY=1 ./bin/context-fabric all
# or: empty POSTGRES_DSN + PROFILE=demo
```

Auth uses the local token adapter (`Authorization: Bearer local:org1:alice:admin`). Point the CLI at the process:

```bash
export CONTEXT_FABRIC_URL=http://127.0.0.1:8080
export CONTEXT_FABRIC_TOKEN=local:org1:alice:admin
```

## Compose

### Demo (local bundled deps)

Requires Docker. Uses `PROFILE=demo`, role `all`, Local auth, and publishes **:8080** directly.

```bash
cp deploy/compose/.env.example deploy/compose/.env
# edit secrets in deploy/compose/.env

make docker-build
make compose-up
# or minimal one-process demo:
docker compose -f deploy/compose/docker-compose.minimal.yaml --env-file deploy/compose/.env up --build
```

### Production starter (split serve/worker + OIDC)

For homelab/production on a single host: bundled Postgres/MinIO/NATS/OpenFGA, **external OIDC**, Caddy TLS on **80/443 only**, `INDEX_BACKEND=postgres`, no `all` role.

```bash
cp deploy/compose/.env.starter.example deploy/compose/.env.starter
# replace all placeholders; configure your IdP

make compose-starter-preflight
make compose-starter-up
```

Runbook: [`docs/operations/deploy-starter-compose.md`](docs/operations/deploy-starter-compose.md).  
Published image: `ghcr.io/mohd-ismail0/gontext` (pin digest in production).

**Index:** starter/xsama/scaled set `INDEX_BACKEND=postgres` so search projections and the AuthZ tuple outbox share Postgres. Demo keeps Local auth + memory OpenFGA; starter/xsama/scaled require real OIDC and OpenFGA (fail-closed).

Ready probes assert pinned OpenFGA model, `RelationshipWriter`, AuthZ outbox health (no dead letters), and migrations including `004_authz_tuple_outbox`.

Useful targets: `make build`, `make test`, `make lint`, `make migrate`, `make doctor`, `make compose-starter-up`, `make release-check`.

Production starter deploy: see [`docs/operations/deploy-starter-compose.md`](docs/operations/deploy-starter-compose.md).

## cf CLI

Operator CLI (`cmd/cf`) talks to the serve HTTP API:

```bash
go build -o bin/cf ./cmd/cf
export CONTEXT_FABRIC_URL=http://127.0.0.1:8080
export CONTEXT_FABRIC_TOKEN=local:org1:alice:admin

cf doctor
cf tenant provision --org org1 --name Acme
cf tenant verify --org org1
cf agent create --org org1 --agent agent1
cf agent rotate --org org1 --agent agent1
cf agent revoke --org org1 --credential <credential_id>
cf source register --org org1 --system chatwoot
cf source verify --org org1 --source <source_id>
cf diagnose decision --org org1 --audit-id <audit_id>
cf ops lag --org org1
cf ops support-bundle --org org1
```

MCP is mounted at `POST /mcp` with the same bearer auth; OAuth protected-resource metadata is at `/.well-known/oauth-protected-resource`.

## Image roles

| Role | Purpose |
|------|---------|
| `serve` | REST, MCP, authn/authz, retrieval, packets, audit, change feed |
| `worker` | Outbox relay, projections, deletion/export jobs |
| `connector` | Isolated source connectors (no DB/NATS/OpenFGA creds) |
| `migrate` / `bootstrap` | Forward migrations; verify schema + grants |
| `doctor` | Preflight connectivity checks (does not override argv role) |
| `backup` / `restore` / `reconcile` | Point to `scripts/*.sh` unless stub ops allowed |
| `all` | Serve+worker in one process (required while index is in-memory) |

One-shot roles (`migrate`, `bootstrap`, `doctor`, `backup`, `restore`, `reconcile`, `connector`) always honor argv and ignore `CONTEXT_FABRIC_ROLE`. Long-running `serve`/`worker`/`all` still prefer the env var when set (Kubernetes).

```bash
docker run --rm ghcr.io/mohd-ismail0/gontext:1.0.0 all
docker run --rm ghcr.io/mohd-ismail0/gontext:1.0.0 migrate
```

## Profiles

Concrete configs under [`deploy/profiles/`](deploy/profiles/) (`demo`, `starter`, `xsama`, `scaled`) validate against [`deploy/schema/config.schema.json`](deploy/schema/config.schema.json). Secrets are classified (never valued) in [`deploy/schema/secrets.schema.json`](deploy/schema/secrets.schema.json).

| Profile | Packaging |
|---------|-----------|
| `demo` | Bundled deps, role `all`; Local auth + memory OpenFGA OK — **not for production** |
| `starter` | Compose bundled PG/NATS/MinIO/OpenFGA; split serve/worker; external OIDC (fail-closed) |
| `xsama` | Coolify + private LXCs; public serve only — see [`deploy/coolify/README.md`](deploy/coolify/README.md) |
| `scaled` | Helm, external deps; use role `all` until durable index |

Helm chart: [`deploy/helm/context-fabric/`](deploy/helm/context-fabric/).

## Architecture pointers

- ADRs: [`docs/adr/`](docs/adr/) — especially [0001 one-image roles](docs/adr/0001-one-image-role-topology.md), [0002 profiles](docs/adr/0002-deployment-profiles.md), [0003 bundled vs external](docs/adr/0003-bundled-vs-external-infra.md), [0012 v1 boundaries](docs/adr/0012-v1-boundaries.md), [0013 knowledge-graph-first](docs/adr/0013-knowledge-graph-first.md), [0015 sparse-first MCP](docs/adr/0015-sparse-first-retrieval-and-portable-mcp.md)
- Contracts: [`contracts/openapi/openapi.yaml`](contracts/openapi/openapi.yaml), [`contracts/jsonschema/context-packet.json`](contracts/jsonschema/context-packet.json), [`contracts/conformance/suite.yaml`](contracts/conformance/suite.yaml)
- Threat model: [`docs/threat-model/`](docs/threat-model/)
- Design reference: [`context-fabric-stress-test.md`](context-fabric-stress-test.md)

## V1 boundaries

**In v1:** employees/service principals/delegated agents; generic CloudEvents intake + MappingSpec; Chatwoot text connector + synthetic conformance connector; REST + read-only MCP (`search`, `get`, `brief`, `graph`, `request_access`); context as a ReBAC-gated knowledge graph (ADR 0013); sparse/FTS-first ranked search with adaptive graph hops (ADR 0015); cursor feed + signed webhooks (metadata only); org export/import; OpenFGA + Go `PolicyProvider`; PostgreSQL/RLS (+ optional pgvector behind a dense-retrieval gate), S3 evidence, NATS JetStream; Compose / Coolify / Helm from one image; `cf` CLI; optional non-authoritative harness skill packs.

**Out of v1:** customer-facing REST/MCP auth; agent write/send; attachment parsing beyond quarantine; third-party plugins; public NATS streams; OPA/Cedar/SpiceDB/Kafka/OpenSearch/Qdrant/Temporal; SCIM as live authz; always-on dense vectorization of evidence.

See [ADR 0012](docs/adr/0012-v1-boundaries.md) and [ADR 0015](docs/adr/0015-sparse-first-retrieval-and-portable-mcp.md).

## License

Licensed under the [Apache License, Version 2.0](LICENSE). See [`NOTICE`](NOTICE) for attribution.
