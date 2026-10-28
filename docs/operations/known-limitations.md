# Known limitations

Items identified during review that are **not yet fixed**, with the reason they
are deferred. Each has a concrete follow-up. Fixes already landed are covered by
tests; see the referenced files.

## Security

### AuthZ objects are not organization-scoped (import path)
OpenFGA objects are named globally (`resource:<id>`, `organization:<id>`) with no
org namespace, and `POST /context/imports` writes edges/`authz_tuples` from a
caller-supplied manifest (`internal/export/export.go`, `ImportInto`). A `manage`
holder could enqueue a tuple referencing another org's resource id once a
same-named local record exists.

**Why deferred:** an existence-based guard cannot distinguish a legitimate
placeholder/not-yet-ingested node (edges are allowed to reference them —
`UpsertEdge` does not create placeholders) from another org's resource. The
correct fix is org-scoped object ids (e.g. `resource:<org>:<id>`) in
`internal/authzsync` + the OpenFGA model — a model change that requires
regenerating `contracts/openfga/model.json` with the `fga` CLI.

### Webhook delivery: residual DNS-rebinding window
Redirect-based SSRF is fixed (delivery never follows redirects,
`internal/changes/changes.go` `safeDeliveryClient`). A target that resolves to a
public IP at registration can still be re-pointed to an internal IP before
delivery. **Follow-up:** pin the resolved address with a custom `DialContext`.

## Test coverage

- No test exercises a real adapter (`postgres`, real OpenFGA, S3, NATS); the
  conformance suite and unit tests use the in-memory adapters. A DB-backed
  integration test must run as the non-superuser `context_gateway` role for RLS
  to be meaningful.
- `internal/conformance` dispatches by case ID; `contracts/conformance/suite.yaml`
  `steps:` are descriptive. A parity test now fails if a YAML case has no handler.
- `internal/adapters/nats`, `internal/httpapi` handler bodies, connectors, and
  `cmd/*` are lightly or not covered.

## Deployment / supply chain

- **MinIO images** were removed from Docker Hub (pull 404s). The server now
  defaults to a verified drop-in mirror (`openebs/minio`, same upstream release
  tag) and the bucket bootstrap uses `amazon/aws-cli` instead of `minio/mc`. Both
  are overridable via `MINIO_IMAGE` / `S3_INIT_IMAGE`.
- Helm `dependencies.*.connectionMode: bundled` is declared but no bundled
  resources are templated, and the `migrate` job is a `pre-install` hook (would
  run before bundled deps exist). External dependencies are required today.
- Container CVE scanning (Trivy/Grype) and SBOM attach/attest to the published
  digest are not wired; the SBOM is uploaded as a build artifact only.
- Secrets are passed via environment variables, not mounted files.
- Postgres `sslmode` now defaults to `disable` but is overridable via
  `POSTGRES_SSLMODE` (compose) / `dependencies.postgres.sslMode` (Helm).

## Code

- `postgres.mustPgTx` panics on an unexpected `Tx` type. This only triggers on a
  programming error (callers always pass a tx from `WithOrgTx`); converting it to
  a returned error is a broad, low-value refactor.
- The change-feed `cursor` column is retained for compatibility but no longer
  used for ordering (events are ordered by `occurred_at, id`).
- Module path is `github.com/xsama/context-fabric` while the repo/image are
  `mohd-ismail0/gontext`. This is **intentional for the current milestone** per
  [ADR 0016](../adr/0016-compose-first-production-release.md).
- Webhook delivery enumerates orgs from the in-memory subscription map
  (`internal/changes/changes.go`), so after a process restart it will not resume
  for orgs whose subscriptions live only in the durable store. The durable store
  has no list-all-orgs API; a fix needs one.
- The in-memory adapter's `WithOrgTx` does not roll back on error (unlike
  Postgres), so tests run against memory cannot catch partial-write bugs.
