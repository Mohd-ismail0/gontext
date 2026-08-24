# Release gates (CI)

These CI gates map to the stress-test / stress-ratify program. A release candidate
must pass every gate below (or document an explicit waiver with expiry).

| Gate | What it proves | Primary artifacts | Automation status |
|------|----------------|-------------------|-------------------|
| **Authz matrix** | Employee, manager, owner, assignee, agent, revoked/expired agent, customer (deny), and cross-org fixtures produce the expected allow/deny | `contracts/authorization-fixtures/*.json`, `go test ./internal/conformance/...` | **Automated** via `TestAuthorizationFixturesAllowDenyMatrix` |
| **Replay** | Duplicate / reordered / delayed intake yields identical canonical revision and idempotent outcome | `contracts/conformance/suite.yaml`, `go test ./internal/conformance/...`, ingest unit tests | **Automated** via in-process suite runner (`TestConformanceSuiteInProcess`, `cf conformance run`) |
| **Tombstone** | Delete revokes visibility; search/get/brief stay empty even if stale index rows reappear; restore must replay tombstones before serve | deletion tests, e2e memory path, `docs/operations/backup-restore.md`, `scripts/restore.sh` | **Partial** — deletion/e2e + export tombstone import automated; restore-order ops remain operator-run via scripts |
| **MCP parity** | MCP `context.search` / `get` / `brief` / `graph` / `request_access` share ApplicationService outcomes with REST | `contracts/conformance/suite.yaml` (`mcp-rest-parity`), MCP server tests | **Automated** via suite runner + MCP unit tests |
| **Graph AuthZ** | Returned edges require both endpoints to survive org isolation, OpenFGA `can_read`, and policy; placeholders never leak | retrieval/graph tests, `graph-visible-subgraph` conformance case | **Automated** via suite runner + unit tests |
| **AuthZ outbox** | Parent `sync_authz` edges enqueue tuples transactionally; worker/reconcile apply without request-path OpenFGA | ingest/worker tests, migration `004`, `authz-outbox-parent-sync` case, `scripts/reconcile.sh` | **Automated** via ingest + worker + suite; reconcile is operator/script |
| **Export** | Org export completes with stable checksums, no secrets, evidence refs, and import/tombstone parity | export tests, e2e export step, `export-round-trip` conformance case | **Automated** via export tests + suite runner |
| **Webhook metadata-only** | Signed change deliveries never include content, vectors, embeddings, prompts, or authorization tuples; retries/duplicates are durable | changes tests, `webhook-retry-idempotent` conformance case | **Automated** via changes tests + suite runner |
| **Tag non-widening** | Caller tags / free labels never grant ACL; classification and visibility remain system fields | `tag-downgrade-deny` fixture, retrieval tag tests, mapping ACL non-broadening case | **Automated** for fixtures, retrieval, and suite mapping case |

## Honesty note

`contracts/conformance/suite.yaml` is the **normative case catalog**. The Go runner
(`internal/conformance`, `cf conformance run`) executes every named case in-process
against memory adapters. Language SDKs and remote sandbox runners may still add
coverage; CI gate is `go test ./internal/conformance/...`.

## How to run locally

```bash
go test ./...
cf conformance run --suite contracts/conformance/suite.yaml
```

Optional focused runs:

```bash
go test ./internal/conformance/...
go test ./internal/application/ -run TestE2EMemoryHappyPathAndRevoke
```

Ops roles (scripts):

```bash
context-fabric backup [out-dir]
context-fabric restore <backup-dir>
context-fabric reconcile
```

## Capacity / SLO

Numeric latency and revocation SLOs are measurement-derived. See
[`slo-baselines.md`](./slo-baselines.md). Until staging soak values exist, CI enforces
correctness gates in this document rather than invented latency budgets.
