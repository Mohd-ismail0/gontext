# Release gates (CI)

These gates map to the production-readiness program ([ADR 0016](../adr/0016-compose-first-production-release.md)).
A release candidate must pass every gate below or document an explicit waiver.

## Waiver policy

| Severity | Waivable for GA? | Required fields |
|----------|------------------|-----------------|
| **P0** (security, recovery, auth fail-closed) | **No** | — |
| **P1** (operability, observability, supply chain) | Yes, time-boxed | owner, compensating control, expiry date |

## P0 — non-waivable (GA blockers)

| ID | Gate | What it proves | Primary artifacts | Automation |
|----|------|----------------|-------------------|------------|
| P0-1 | **Starter fail-closed auth** | Profile `starter` rejects local tokens, memory OpenFGA, placeholder secrets, and missing OIDC | `internal/config`, `cmd/context-fabric/main.go`, Compose starter overlay | Integration + compose smoke |
| P0-2 | **Admin authz** | Mutations require `context:manage_policy` + OpenFGA `can_manage`; intake requires `context:ingest` | `internal/application/authz_gate.go`, service tests | Unit + integration |
| P0-3 | **Authz matrix** | Employee, manager, owner, assignee, agent, revoked/expired agent, customer (deny), cross-org | `contracts/authorization-fixtures/*.json` | `go test ./internal/conformance/...` |
| P0-4 | **Webhook SSRF** | HTTPS-only destinations outside demo; private/metadata IPs blocked | `internal/changes/webhook_validate.go` | `internal/changes/*_test.go` |
| P0-5 | **Secrets required** | `WEBHOOK_SIGNING_SECRET`, `DELETION_SIGNING_SECRET` set for non-demo; no fallback signing material | `cmd/context-fabric/main.go` | Startup + doctor |
| P0-6 | **Migration parity** | `migrations/` matches embedded `internal/migrate/migrations/` | CI diff | Automated |
| P0-7 | **Restore drill** | Host-managed backup/restore meets RPO ≤30m / RTO ≤60m; tombstones survive restore | `scripts/backup.sh`, `scripts/restore.sh`, runbook | Manual quarterly + CI optional |
| P0-8 | **Signed digest** | Published image cosign-verified, SBOM attached, zero unwaived CRITICAL CVEs | `.github/workflows/release.yml` | Release workflow |

## P1 — correctness and operability (waivable with expiry)

| ID | Gate | What it proves | Primary artifacts | Automation |
|----|------|----------------|-------------------|------------|
| P1-1 | **Replay** | Duplicate/reordered intake is idempotent | `contracts/conformance/suite.yaml` | Conformance suite |
| P1-2 | **Tombstone** | Delete → empty search; restore replays tombstones | deletion tests, e2e | Partial + restore script |
| P1-3 | **MCP parity** | MCP tools match REST ApplicationService outcomes | `mcp-rest-parity` case | Conformance suite |
| P1-4 | **Graph AuthZ** | Visible subgraph respects org + OpenFGA + policy | `graph-visible-subgraph` | Conformance suite |
| P1-5 | **AuthZ outbox** | Parent edges enqueue tuples; worker applies; dead=0 at ready | migration `004`, worker tests | Ready probe + conformance |
| P1-6 | **Export** | Stable checksums, no secrets, import/tombstone parity | export tests | Conformance suite |
| P1-7 | **Webhook metadata-only** | Deliveries never include content/tuples; retries idempotent | changes tests | Conformance suite |
| P1-8 | **Tag non-widening** | Tags never broaden ACL | fixtures + mapping case | Conformance suite |
| P1-9 | **Compose smoke** | Starter stack reaches `/health/ready` with OIDC token search | `scripts/compose-smoke.sh` | CI compose-smoke job |
| P1-10 | **Remote conformance** | Full suite against live stack (not memory only) | `internal/conformance/remote.go` | Integration workflow |
| P1-11 | **Helm valid** | `helm lint`, values schema, rendered manifests | `deploy/helm/context-fabric/` | CI helm job |

## Regression baselines (must not regress)

- Migration embed parity (`diff -qr migrations internal/migrate/migrations`)
- FORCE RLS + NOBYPASSRLS gateway role
- AuthZ tuple outbox with ready-probe dead-letter check
- Non-root runtime (UID 65532)
- In-process conformance suite green on every PR

## Required CI jobs (protected checks)

| Job | Scope |
|-----|-------|
| `lint` | go vet, golangci-lint (if configured) |
| `unit` | `go test -race -count=1 ./...` |
| `contracts` | JSON Schema, OpenAPI, OpenFGA, profile YAML validation |
| `integration` | Live Postgres/MinIO/NATS/OpenFGA/Dex (Linux only) |
| `compose-smoke` | Pull/build image, starter overlay, health + OIDC smoke |
| `security` | govulncheck, Trivy image scan |
| `release` | Multi-arch push, cosign, SBOM (tags only) |

## How to run locally

```bash
make release-check    # aggregate pre-release gates
go test -race -count=1 ./...
go test -tags=integration ./...   # requires integration stack
cf conformance run --suite contracts/conformance/suite.yaml
make compose-starter-smoke
```

## Capacity / SLO

Numeric latency and revocation SLOs are measurement-derived. See
[`slo-baselines.md`](./slo-baselines.md). CI enforces correctness gates until the 72h soak fills SLO tables.
