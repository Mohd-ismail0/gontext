# Release gates (CI)

These CI gates map to the stress-test / stress-ratify program. A release candidate
must pass every gate below (or document an explicit waiver with expiry).

| Gate | What it proves | Primary artifacts |
|------|----------------|-------------------|
| **Authz matrix** | Employee, manager, owner, assignee, agent, revoked/expired agent, customer (deny), and cross-org fixtures produce the expected allow/deny | `contracts/authorization-fixtures/*.json`, `go test ./internal/conformance/...` |
| **Replay** | Duplicate / reordered / delayed intake yields identical canonical revision and idempotent outcome | `contracts/conformance/suite.yaml` (`intake-parity-single-and-batch`, `webhook-retry-idempotent`), ingest unit tests |
| **Tombstone** | Delete revokes visibility; search/get/brief stay empty even if stale index rows reappear; restore must replay tombstones before serve | deletion tests, e2e memory path, `docs/operations/backup-restore.md` |
| **MCP parity** | MCP `context.search` / `get` / `brief` / `request_access` share ApplicationService outcomes with REST | `contracts/conformance/suite.yaml` (`mcp-rest-parity`), MCP server tests |
| **Export** | Org export completes with stable checksums, no secrets, and import/tombstone parity | export tests, e2e export step, export-round-trip conformance case |
| **Webhook metadata-only** | Signed change deliveries never include content, vectors, embeddings, prompts, or authorization tuples | changes tests, `webhook-retry-idempotent` conformance case |
| **Tag non-widening** | Caller tags / free labels never grant ACL; classification and visibility remain system fields | `tag-downgrade-deny` fixture, retrieval tag tests, mapping ACL non-broadening case |

## How to run locally

```bash
go test ./...
```

Optional focused runs:

```bash
go test ./internal/conformance/...
go test ./internal/app/ -run TestE2EMemoryHappyPathAndRevoke
```

## Capacity / SLO

Numeric latency and revocation SLOs are measurement-derived. See
[`slo-baselines.md`](./slo-baselines.md). Until staging soak values exist, CI enforces
correctness gates in this document rather than invented latency budgets.
