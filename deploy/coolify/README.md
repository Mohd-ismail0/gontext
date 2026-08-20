# XSAMA Coolify deployment notes (profile: xsama)

Context Fabric on Coolify uses the **same signed image** as Compose/Helm. Roles run as separate Coolify applications or process commands (`serve`, `worker`, `connector`). Dependencies are **external** on dedicated private LXCs. Only `serve` is public.

Profile source of truth: [`../profiles/xsama.yaml`](../profiles/xsama.yaml) (ADR 0002 / 0003).

## Topology

| Component | Placement | Exposure |
|-----------|-----------|----------|
| `context-fabric serve` | Coolify application VM / container | **Public** HTTPS (8080 behind Coolify proxy) |
| `context-fabric worker` | Same Coolify project, private | Private only |
| `context-fabric connector` | Isolated Coolify service | Private egress to sources + intake; **no** DB/NATS/OpenFGA creds |
| PostgreSQL + pgvector | LXC (e.g. 103) | Private cross-LXC only |
| SeaweedFS / S3 | LXC (e.g. 108) | Private cross-LXC only |
| NATS JetStream | Dedicated private service/LXC | Private cross-LXC only |
| OpenFGA | Private Coolify service or LXC | Private cross-LXC only |
| OIDC (Logto) | Existing IdP | Public discovery/JWKS; generic OIDC adapter only |

## Public surface rule

- Publish **only** the `serve` HTTP port through Coolify's proxy.
- Do **not** publish PostgreSQL `5432`, MinIO/SeaweedFS S3 ports, NATS `4222`/`8222`, or OpenFGA `8080`/`8081` to the internet or untrusted VLANs.
- MCP and REST share the same `serve` process; do not deploy a second public data plane.

## Cross-LXC networking checklist

1. **DNS** — Resolvable private names for `postgres`, `nats`, `s3`/`seaweedfs`, and `openfga` from the Coolify app LXC (match `deploy/profiles/xsama.yaml` endpoints).
2. **Firewall** — Allow Coolify app → data LXCs on required ports only; deny data LXC inbound from WAN; deny lateral access from connector to Postgres/NATS/OpenFGA.
3. **TLS** — Prefer TLS for Postgres, S3, NATS, and OpenFGA across LXC boundaries; mount CA material via Coolify secrets (`*_CA` / secret refs in profile).
4. **Credentials** — Store secrets in Coolify env/secret store; classifications in [`../schema/secrets.schema.json`](../schema/secrets.schema.json). Connector secrets must not include ledger/JetStream/OpenFGA credentials.
5. **Capabilities** — Before traffic: run `context-fabric doctor` and confirm `pgvector`, JetStream file persistence, S3 versioning + object lock, OpenFGA store/model, OIDC issuer/audience.
6. **Migrations** — Run `migrate` then `bootstrap` as one-shot Coolify jobs/commands. Never enable auto-migrate on `serve` start.
7. **Health** — Proxy readiness to `/health/ready`; liveness to `/health/live`; startup to `/health/startup`.
8. **Backup ownership** — Shared between platform and operator per xsama profile: Postgres dumps, OpenFGA model/tuples, S3 version manifests, config + secret refs. JetStream may be rebuilt from outbox after restore + `reconcile`.
9. **Clock skew** — NTP on all LXCs; doctor fails closed on excessive skew.
10. **OIDC** — Point generic OIDC settings at Logto (or any compliant IdP). No Logto-specific imports in core.

## Durable index and AuthZ outbox

Set `INDEX_BACKEND=postgres` for Coolify `serve`/`worker` splits so search projections and `authz_tuple_outbox` share Postgres. Ready probes require a pinned OpenFGA model, a `RelationshipWriter`, migrations through `004_authz_tuple_outbox`, and no dead-letter AuthZ tuples.

## Suggested Coolify process commands

```text
serve:      /usr/local/bin/context-fabric serve
worker:     /usr/local/bin/context-fabric worker
connector:  /usr/local/bin/context-fabric connector
migrate:    /usr/local/bin/context-fabric migrate
bootstrap:  /usr/local/bin/context-fabric bootstrap
doctor:     /usr/local/bin/context-fabric doctor
```

Image: `ghcr.io/xsama/context-fabric:<version>` (multi-arch amd64/arm64, non-root).

## Acceptance bar

- Starter/XSAMA install with real OIDC reaches authenticated search without manual SQL, OpenFGA UI clicks, or container shell access.
- `doctor` is green before the Coolify proxy marks `serve` healthy.
