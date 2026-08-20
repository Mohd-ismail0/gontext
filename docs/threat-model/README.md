# Context Fabric Threat Model (STRIDE)

## Scope

v1 surfaces: `serve` (REST/MCP), `worker`, `connector`, bootstrap/doctor, PostgreSQL/RLS, OpenFGA, NATS JetStream, S3 evidence, OIDC, change feed/webhooks, export/import, `cf` CLI.

## Assets

- Canonical ledger and evidence (source of truth)
- Derived projections (rebuildable)
- Authorization model/tuples and delegation grants
- Agent API keys and source/webhook secrets
- Cited context packets (disclosure surface)
- Audit trails (integrity and non-repudiation)

## Trust boundaries

1. Public internet → `serve` only
2. `serve` → private PostgreSQL / OpenFGA / NATS / S3
3. Connector process → public intake APIs only (no DB)
4. MCP client → OAuth resource server (`serve`)
5. Telemetry exporters → redacted OTLP only

## STRIDE summary

| Component | Spoofing | Tampering | Repudiation | Info disclosure | DoS | Elevation |
|-----------|----------|-----------|-------------|-----------------|-----|-----------|
| OIDC/JWT | JWKS rotation, aud/iss/azp checks | — | audit with jti hash | no tokens in logs | JWKS cache | org path must match token |
| Delegation | key→agent principal only | grant rows append-only revoke | actor_chain in audit | — | grant TTL | intersection with creator authority |
| OpenFGA | mTLS/service auth | pinned model ID | decision in audit | fail closed | concurrency limits | no tuple admin on public API |
| RLS | — | FORCE RLS, NOBYPASSRLS | — | SET LOCAL per txn | pool exhaustion quotas | fail if org GUC unset |
| Intake | source HMAC/signature | MappingSpec cannot broaden ACL | event_id uniqueness | no bodies on NATS | size/rate quotas | trust ceiling on source |
| Connectors | digest pin, no DB creds | SBOM/sign | connector audit | egress allowlist | CPU/mem limits | capability manifest |
| Retrieval | — | Batch Check before hydrate; visible-subgraph filter | citations+audit_id | ID-only index; redaction; no placeholder leak | candidate bounds | tags cannot widen |
| Knowledge graph | — | edge+AuthZ outbox same txn | edge change events | dangling edges filtered | depth/node budgets | visibility_ref ≠ ACL |
| AuthZ sync | service auth to OpenFGA | pinned model; outbox+reconcile | audit on apply failures | no tuples on public API | outbox lag readiness | no post-commit request-path writes |
| MCP | PRM + audience | no token passthrough | parity audit | read-only tools | scope minimal | confused-deputy blocked |
| Change feed | webhook HMAC | opaque IDs only | delivery receipts | no content in events | retry/DLQ | hydrate via gateway |
| Deletion | authorize+legal hold | tombstone dominates | signed completion | — | — | restore replays tombstones |
| Telemetry | — | — | — | allowlist redaction | — | never auth from baggage |
| Export | org-scoped authz | checksums | job audit | no secrets in export | job quotas | legal hold respected |

## Abuse cases (must fail closed)

1. Tag downgrade grants access
2. Agent retains creator rights after role revoke
3. Leaked API key used as human identity
4. Prompt injection causes cross-context read or send
5. Webhook replay corrupts ledger
6. Compromised connector reaches DB/NATS
7. OpenFGA down → stale broadened allow
8. Incomplete delete leaves vectors searchable
9. Content-bearing public webhook
10. Cross-tenant RLS pool leak

## Mitigations ownership

Gateway ApplicationService owns all disclosure decisions. Workers rebuild projections only. Connectors submit intake only. Operators use `cf` control APIs, never raw SQL/OpenFGA UI for routine work.