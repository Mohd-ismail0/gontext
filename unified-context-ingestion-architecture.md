# Unified Organizational Context Platform — Reference Architecture

**Scope:** Self-hosted, multi-tenant organizational context: customer-facing and internal sources; durable ingestion; provenance; agent/API/MCP retrieval; RBAC; deletion/redaction; audit. This is an architecture recommendation, **not** a deployment plan.

## Executive recommendation

Use **PostgreSQL as the transactional system of record and provenance/ACL control plane**, **Kafka-compatible event streaming as the durable replayable data plane**, **S3-compatible object storage as immutable raw evidence**, **Temporal as the long-running workflow controller**, and **ClickHouse as the high-volume retrieval projection**. Treat every derived artifact (extracted text, document element, chunk, embedding, summary, search index row) as a replaceable projection of a versioned source artifact, never as the canonical truth.

For this stack, start with:

- **Apache Kafka in KRaft mode + Debezium + Apicurio Registry** if strict OSI licensing and the broad Kafka Connect/CDC ecosystem are priorities. Kafka’s idempotent producer and transactions support exactly-once *within the log-processing boundary*; a producer transaction atomically writes records and consumer offsets.[60]
- **Redpanda** is a reasonable Kafka-API alternative if simpler operations and a single-binary Kafka-compatible broker outweigh licensing constraints. Its community edition is source-available under BSL, not Apache-2.0, and may not be offered as a commercial streaming/queuing service.[61][63]
- **NATS JetStream** is excellent for low-latency internal command/work queues and service signaling, but not the primary organizational event ledger here: its documented base delivery semantic is at-least-once, and its Connect/CDC/schema-governance ecosystem is materially narrower.[64]
- Keep the existing **SeaweedFS** for bulk S3-compatible storage only after validating its exact version/locking behavior against required retention semantics. If defensible WORM/legal-hold retention is a firm requirement, choose an S3 implementation/version with verified Object Lock behavior (MinIO AIStor documentation describes versioned WORM and legal holds) rather than assume generic S3 compatibility is sufficient.[69]

All core recommendations except Redpanda are permissively licensed/open source: Debezium, Unstructured, Tika, Apicurio, ClickHouse, OTel Collector, OPA, OpenFGA and SeaweedFS are Apache-2.0; Temporal server is MIT. Validate dependency/commercial feature licensing at procurement time.

## Logical topology

```text
                    ┌─────────────────────────────────────┐
Sources / Connectors│ SaaS APIs, IMAP, webhooks, uploads,  │
                    │ CRM/helpdesk, repositories, DB apps │
                    └──────────────┬──────────────────────┘
                                   │ authenticated intake
                    ┌──────────────▼──────────────────────┐
                    │ Context API / connector adapters     │
                    │ Postgres transaction: source state + │
                    │ append-only outbox                   │
                    └───────┬─────────────┬───────────────┘
                            │             │
                  raw bytes │             │ CDC (only outbox)
                            │             ▼
                 ┌──────────▼───┐  ┌──────────────────────┐
                 │ S3 object    │  │ Debezium → Kafka /   │
                 │ store         │  │ Redpanda event log   │
                 │ raw, rendered │  │ schema registry       │
                 └──────────────┘  └─────────┬────────────┘
                                              │
                             durable work commands / state events
                                              ▼
                    ┌─────────────────────────────────────┐
                    │ Temporal workflows                  │
                    │ fetch → scan → extract → chunk →    │
                    │ embed → index / redact / delete     │
                    └─┬──────────────┬──────────────┬──────┘
                      │              │              │
          versioned outputs    extracted elements    projections
                      │              │              │
                ┌─────▼────┐   ┌─────▼─────┐  ┌────▼─────────┐
                │ S3       │   │ PostgreSQL│  │ ClickHouse    │
                │ evidence │   │ catalog,  │  │ lexical + ANN │
                │ / hashes │   │ lineage,  │  │ retrieval     │
                └──────────┘   │ ACL state │  └────┬─────────┘
                               └───────────┘       │
                    ┌──────────────────────────────▼────────┐
                    │ Retrieval Gateway / Agent API / MCP    │
                    │ AuthN → OpenFGA authorization → query  │
                    │ ACL-filtered candidate retrieval →     │
                    │ citation/provenance hydration           │
                    └────────────────────────────────────────┘

Cross-cutting: OpenTelemetry Collector + audit-event stream; OPA for policy
admission/redaction/retention decisions.
```

## 1. Canonical data model and immutable event contract

### Separate four identities

1. **Logical resource** — stable `resource_id`: e.g., a Zendesk ticket, Drive file, or uploaded document.
2. **Observed revision** — immutable `revision_id` / content hash: a particular retrieved version of that resource.
3. **Derived artifact** — `artifact_id` + `derivation_id`: extracted-text version, element set, chunks, embedding set, summary, classification.
4. **Event** — immutable `event_id`: one fact about a state transition or processing attempt.

Never use URLs, external IDs, chunk IDs, or a mutable `updated_at` as a globally unique event identity. Use UUIDv7/ULID event IDs and stable deterministic IDs only where replay is intended (for example, `chunk_id = hash(revision_id, extraction_profile, element_locator, chunk_profile, ordinal)`). Keep content SHA-256 plus byte length/MIME detection result for every raw/retrieved blob.

### Standard envelope

Adopt CloudEvents 1.x as the transport envelope; CloudEvents exists specifically to standardize event declaration/delivery across sources and has broad SDK support.[4] Use a small stable, schema-registry-governed payload and extensions, e.g.:

```json
{
  "specversion": "1.0",
  "id": "evt_01...",
  "source": "urn:context:connector:zendesk:tenant:acme",
  "type": "com.example.context.document.revision.observed.v1",
  "subject": "resource/doc_01.../revision/rev_01...",
  "time": "2026-08-19T12:34:56Z",
  "datacontenttype": "application/json",
  "dataschema": "apicurio://context/document-revision/12",
  "tenantid": "tenant_01...",
  "classification": "internal",
  "traceparent": "00-...",
  "data": {
    "resource_id": "doc_01...",
    "revision_id": "rev_01...",
    "content_ref": {"uri": "s3://context-raw/...", "version_id": "...", "sha256": "..."},
    "source_cursor": {"connector": "zendesk", "external_id": "...", "cursor": "..."},
    "actor_ref": "service:connector-zendesk",
    "observed_at": "..."
  }
}
```

**Required every time:** `event_id`, event type/version, tenant/workspace, logical resource/revision, immutable source evidence reference and hash (when content exists), producer, occurred/observed time, correlation/causation IDs, classification, schema ID/version, and `traceparent`. Do not place raw PII, document body, OAuth tokens, or secret-bearing source URLs in event payloads/logs.

### Event categories and topics

Use bounded topic families rather than one topic per tenant:

- `context.lifecycle.v1` — `resource.discovered`, `revision.observed`, `access.changed`, `delete.requested`, `redaction.requested`, `purge.completed`.
- `context.work.v1` — commands to perform extraction/indexing; compact/retain conservatively only after audit need is separate.
- `context.derived.v1` — `extraction.completed`, `chunks.materialized`, `embeddings.materialized`, `projection.applied`, `failed`.
- `context.audit.v1` — append-only access, policy, administration, export, and deletion decisions.
- `context.dlq.v1` — envelope + failure classification + pointer, never an uncontrolled raw-content dumping ground.

Partition lifecycle events by `(tenant_id, resource_id)` so a resource’s transitions remain ordered. Do not rely on global ordering. Make retention an explicit contract per topic: raw/audit longer than derived work results; compacted “current state” topics must not replace the immutable audit/lifecycle log.

## 2. Ingest, outbox/CDC, and idempotency

### Recommended transaction boundary

For connector/API intake, persist source state and an outbox row in **one PostgreSQL transaction**:

- upsert the source cursor/resource/revision metadata;
- upload raw bytes first to a temporary/quarantine object key, scan/validate, then promote to a content-addressed immutable key; record the exact object version/hash;
- insert `outbox_event(event_id PK, aggregate_key, type, schema_ref, payload, occurred_at, trace_context)` in the same database transaction as the catalog mutation.

Then Debezium reads **only the outbox table** and routes it to the broker. Debezium documents the outbox pattern as avoiding inconsistencies between service database state and consumed event state; its Outbox Event Router routes an outbox table and propagates the unique event ID as a header that can be used to remove duplicates.[57] PostgreSQL logical slots are crash-safe but may resend recent changes after restart because slot state is persisted at checkpoints, so every downstream consumer must tolerate duplicate delivery.[59]

**Do not dual-write** “commit PostgreSQL then publish broker” in app code. The classic crash gap is unavoidable without the outbox. Also do not use Debezium CDC of every operational table as the public domain-event contract: table-shape changes leak as API changes, snapshots are harder to reason about, and consumers become coupled to storage internals. Use raw CDC selectively for integration/reconciliation; use outbox events for contractually meaningful transitions.

### At-least-once end-to-end, idempotent effects

There is no credible, general “exactly once” guarantee across PostgreSQL, broker, S3, OCR/model calls, ClickHouse, and external connector APIs. Aim for **at-least-once transport + deterministic/idempotent side effects + auditable reconciliation**.

Each materializer owns a durable Postgres `processed_event` (or unique projection key) ledger. In one short transaction, it claims `(consumer_name,event_id)` with a unique constraint, applies/records the desired state, and commits. A duplicate inserts nothing and becomes a no-op. For external calls, derive a stable idempotency key from `event_id`/Temporal activity identity and record the external result before acknowledging. This matters because Temporal retries activities and explicitly recommends idempotent activities, particularly writes.[65]

Broker idempotent producers/transactions complement this design but do not eliminate consumer-side idempotency: Redpanda documents that producer idempotency is session-scoped, and manual app retries can get a new producer ID and produce duplicates.[62] Redpanda/Kafka transactions atomically connect consumed offsets and produced records *inside the broker*, not writes to S3/ClickHouse/LLM APIs.[61][60]

### Replay and reconciliation

- Every projector must be rebuildable from retained lifecycle events plus S3 evidence.
- Store `event_id`, upstream topic/partition/offset, content version/hash, pipeline profile/version, model version, and derived artifact ID on every output.
- Provide a reconciler that compares catalog desired state to source-of-truth projections and emits repair commands—never silently fixes records without an event/audit entry.
- Keep producer/consumer DLQs as observable work queues: retryable vs permanent failure, attempt count, error class, original event pointer. Require a remediation/replay decision event.

## 3. Schema evolution and compatibility

Use **Apicurio Registry** (Apache-2.0) with Avro or Protobuf for high-volume event payloads; JSON Schema is suitable for externally validated MCP/API documents. Registry rules support validity, compatibility, and integrity; Avro/Protobuf/JSON Schema have full compatibility-rule support.[70]

Policy:

1. Namespace schema subjects by product/domain, not individual deployment.
2. Default to `BACKWARD_TRANSITIVE` for event values. Producers only make additive optional changes; consumers ignore unknown fields.
3. Never reuse field numbers/names with a new meaning. Never mutate semantics under the same event type/version.
4. Breaking changes require `type.v2`, a new topic/subject or explicit migration workflow, dual publication/projection, and a deprecation window—not a compatibility rule set to `NONE`.
5. Version **pipeline profiles** separately from event schemas: parser/version, OCR settings, chunk policy, embedding model/dimension, redaction policy, ACL projection version. Model/chunk changes create new derivations; they do not overwrite history.
6. Enforce schema registration and compatibility in CI; broker consumers reject unknown/malformed schemas to a quarantined DLQ rather than guessing.

## 4. Content processing and derived artifacts

### Workflow boundary

Temporal orchestrates per-`revision_id` ingestion as a durable state machine, not the event bus itself. Workflow stages:

1. Validate envelope, tenant/source authorization, size/MIME/allowlist; create quarantine record.
2. Malware/unsafe archive checks; deduplicate raw bytes by hash without cross-tenant disclosure.
3. Detect MIME; extract normalized text/elements; OCR only when needed.
4. Validate quality (empty, encoding failure, extraction coverage, language, parser warnings), route to manual/quarantine if below threshold.
5. Redact/classify according to policy; store a protected original and a retrieval-safe rendition according to retention policy.
6. Chunk and embed the safe rendition.
7. Atomically write catalog record/status then materialize retrieval projections; emit derived event.
8. Make revision searchable only after ACL + provenance + projection checks complete.

Temporal is appropriate because it provides durable, retrying activities, but activities must remain idempotent and should heartbeat/poll for lengthy work.[65] Do not run a forever-growing “per tenant” workflow: Temporal limits a workflow event history to 51,200 events or 50 MB; use one workflow per revision/job and Continue-As-New for a long coordinator.[92][93]

### Extraction components

- **Apache Tika:** baseline MIME detection and text/metadata extraction across 1,000+ types, a useful deterministic fallback and validation layer.[68] Sandbox it: parsers process hostile office/PDF/archive data. Enforce file/expanded-archive/page/time/memory limits, run least privilege/no network, patch aggressively. Tika 3.3.2 tightened server defaults, requiring an explicit opt-in for several insecure endpoints—do not broadly expose a parser server.[68]
- **Unstructured OSS:** primary structure-aware partitioning/chunk preparation where document layout, titles/tables, page/element metadata, and extraction provenance matter. It chunks post-partition semantic elements rather than blind character windows; `by_title` preserves sections, while large elements/tables are split separately.[67]
- **OCR/layout alternatives:** select behind an internal `Extractor` interface; benchmark actual customer document corpus. Image-only PDFs, tables, multi-column layouts, handwriting, and multilingual data are quality/SLA risks. Persist extractor name/version/settings and quality evidence, not only final text.

### Chunking and embeddings

Do not pick a universal “N tokens + overlap” constant. Keep chunking deterministic and profile versioned:

- Default to structured `by_title` / element-aware chunks; preserve `page`, title path, source element IDs, character offsets and table/image locators. Unstructured exposes hard/soft size controls and warns that global overlap can pollute otherwise clean semantic chunks.[67]
- For tables, preserve a CSV/HTML/structured representation and a human-readable text rendition; do not flatten away cell context.
- Store original extraction separately from redacted/retrieval rendition. Index only policy-allowed text.
- Embed in batches, with an immutable `(embedding_model, revision, dimensions, normalization, chunk_profile)` identity. On model or chunk-policy change, create a parallel embedding generation and switch an alias only after evaluation/backfill.
- Hash redacted chunk text plus profile parameters for deterministic cache keys; hash does not substitute for an event ID/provenance.

## 5. Storage, deletion/redaction, provenance, and audit

### Storage planes

| Plane | Recommended authority | Contents | Mutability / retention |
|---|---|---|---|
| Catalog/control | PostgreSQL | resources, revisions, derivations, source cursors, ACL refs, legal holds, deletion jobs, event receipts | transactional; history append-only where required |
| Evidence | existing SeaweedFS S3 or verified Object-Lock-capable S3 | encrypted raw source, rendered/redacted content, parser outputs, manifests | versioned; evidence policy differs from erasure policy |
| Durable events | Kafka/Redpanda | immutable lifecycle/audit/derived events | topic-specific retention; replicated; replayable |
| Search projection | ClickHouse | safe chunks, embeddings, lexical fields, filters, provenance pointers | fully rebuildable; non-authoritative |
| Workflow history | Temporal persistence + archive | execution state and operational history | retention limited; archive only as operational record |

### Deletion and redaction semantics

Implement deletion as a **saga with an explicit status ledger**, not as a single physical `DELETE`:

1. Authorize request; create `deletion_requested`/`redaction_requested` event with scope, legal-hold check, requester, policy version, and deadline.
2. Immediately revoke retrieval eligibility at the gateway/catalog (deny-by-default) and emit an access-revocation event.
3. Enumerate all source revisions, S3 versions, extracts, chunks, embeddings, summaries/caches, exports, ClickHouse rows, and backups/archives by provenance graph.
4. Quarantine/purge derived search projections first; verify zero retrievable rows with the affected `(tenant, resource, revision)` predicate.
5. Delete or cryptographically erase raw/source content according to contractual retention/legal hold; record each target/outcome. If immutable retention legally prevents physical erasure, report that fact and retain only a non-content tombstone/erasure certificate where policy permits.
6. Emit immutable `deletion_completed` or `deletion_blocked` event with a signed manifest of artifacts/copies checked; periodically reconcile.

A crucial design tension: WORM storage protects historical versions but can prevent immediate physical deletion. MinIO’s documented Object Lock requires versioning, protects individual versions, and an unversioned delete creates a **delete marker** while locked versions remain protected.[69] Do not claim a deletion request has erased bytes merely because the current S3 key returns 404. Partition evidence retention from customer erasure obligations, define legal-hold precedence, and consider per-tenant envelope encryption with key destruction for data that must become irrecoverable while immutable blobs remain.

ClickHouse is projection storage, so never make its asynchronous mutations the deletion source of truth. Delete mutations rewrite immutable data parts asynchronously, and concurrent reads may observe a mix of old/new parts.[91] Gate reads at the retrieval service using `visibility_state != active`/catalog authorization immediately, then wait for and verify physical projection cleanup. Avoid raw direct ClickHouse access for agents/users.

### Provenance manifest

For every citation-capable chunk, store an immutable manifest:

```text
chunk_id, tenant_id, resource_id, revision_id, source_connector/external_id,
raw_object_uri + version_id + sha256, retrieved_at, source_modified_at,
extractor + version + config hash, extraction artifact hash,
page/section/element locator + character offsets, redaction policy/version,
chunk profile/version/hash, embedding model/version/dimensions/hash,
ACL snapshot/reference, event_id, causation_id, pipeline run/workflow ID,
created_at, superseded/revoked/deleted state.
```

The response API/MCP tool should return snippets only with this provenance and a source locator; agents must cite `resource/revision/page/section`, not a chunk’s opaque vector ID. Preserve both `valid_time` (source said) and `system_time` (we observed/processed) to reason about corrections and late-arriving syncs.

### Audit

Emit append-only `context.audit.v1` events for: authentication, source connection/consent changes, access/denial decisions, retrieval candidate/result counts (not plaintext sensitive content), document export/share, policy/model/schema changes, operator actions, deletion/redaction execution, and break-glass actions. Send an independent copy to a governed immutable store and a queryable audit projection; neither is a substitute for the other.

Use OPA decision logs sparingly and with masking. OPA can report decision logs including policy bundle revision, decision ID, W3C trace/span IDs, policy input/result, and fields erased/masked.[8] Configure masks/drop rules before enabling; otherwise access-control logs can become a second PII leak.

## 6. Retrieval, agents/API/MCP, RBAC

Build one **retrieval gateway** used by the product API, internal tools, and MCP server. Do not expose ClickHouse, S3, or the broker directly to agents.

1. Authenticate workload/user, resolve tenant and request purpose.
2. Obtain accessible resource IDs / scopes from **OpenFGA** (`ListObjects` for document-readable resources, pinning an authorization model ID); OpenFGA models are immutable and official guidance recommends pinning the model ID in production.[76][94]
3. Enforce policy (OPA) for purpose/classification/export/redaction conditions. Use OpenFGA for relationship graph authorization; use OPA for attribute-based rules and policy-as-code, not as duplicate relationship store.[77]
4. Query ClickHouse with mandatory tenant, active visibility, classification and **authorization-derived resource filters before/with lexical/vector candidate selection**, then rerank. Apply post-filter as defense in depth but do not obtain global top-K then filter—this can cause recall failures and risks leaks through scores/counts.
5. Hydrate source/provenance from PostgreSQL, recheck state/ACL immediately before response, redact as needed, emit audit decision, return citations.

ClickHouse is a suitable projection because it supports vector similarity indexes (available from 25.8) and full-text indexes (GA from 26.2) alongside structured filtering.[71][72] Treat its indexes as evolving implementation details: run hybrid lexical+ANN retrieval with offline tenant-isolation/recall/latency tests; provision memory/I/O for ANN, and retain a brute-force/lexical fallback per small corpus. ClickHouse RBAC/row policies can be defense-in-depth for a read-only gateway role, but its own documentation says row policies are appropriate only for readonly users and warns against query-plan serialization for users needing enforcement.[73] PostgreSQL RLS should protect catalog/control tables; it default-denies when enabled without a policy, but owners/superusers/BYPASSRLS roles normally bypass it—use distinct non-owner app roles and `FORCE ROW LEVEL SECURITY` where appropriate.[22]

MCP controls: separate read/search/get-source from write/connect/delete tools; narrow OAuth scopes, explicit tenant context (never agent-provided alone), field-level output shaping, per-tool and per-tenant quota, allowlisted sources, confirmation/approval for write or broad-export operations, and tamper-evident audit IDs in every tool response. Treat retrieved content as untrusted instructions and enforce tool-level authorization independently of model behavior.

## 7. Component evaluation

| Component | Fit | Recommendation / caveat |
|---|---|---|
| **Kafka (KRaft)** | Strong primary immutable event log; Kafka Connect ecosystem; Debezium first-class | **Preferred strict-OSS path.** KRaft has been production-ready since 3.3.x.[95] Use 3+ brokers for fault tolerance, RF=3/min ISR 2, TLS/SASL ACLs, quotas, topic ownership and backup/restore drills. Operationally heavier than NATS/Redpanda. |
| **Redpanda** | Strong primary event log if Kafka compatibility/simpler operations desired | **Viable alternative.** Kafka-compatible transactions and idempotent producers; its docs note EOS requires transactions + idempotency and remote recovery has an atomicity caveat.[61] Community license is BSL/source-available, so perform license review.[63] |
| **NATS JetStream** | Excellent lightweight internal messaging/work signaling | **Secondary bus, not source ledger.** Persistent/replayable/replicated, but docs characterize delivery as at-least-once.[64] Use durable pull consumers, explicit ACKs, duplicate window/message IDs, and poison-message advisory capture. Avoid dual primary buses unless responsibilities are crisp. |
| **Debezium PostgreSQL** | Best outbox/CDC bridge | **Adopt.** Monitor connector offset, lag, replication slot health, WAL disk headroom; slots retain WAL during outages.[58] The DB can resend after crash, so idempotent consumers remain mandatory.[59] |
| **Temporal** | Long-lived, retryable document/delete workflows | **Adopt for orchestration.** Not a batch queue or permanent source ledger. Size workflow granularity/history; self-hosted archival is documented experimental and unsupported via Docker, so validate before using it as compliance storage.[66] |
| **Unstructured OSS** | Semantic partitioning and chunks | **Adopt behind abstraction.** Retain element metadata and test against corpus. Pin/scan extra dependencies and separate CPU/GPU/OCR capacity. |
| **Apache Tika** | Broad safe baseline extraction/detection | **Adopt as fallback/validation.** Sandboxed parser service, resource caps, no public exposure. |
| **SeaweedFS** | Existing scalable object store | **Keep for raw/derived content if S3 versioning and retention behavior meet tests.** Do not infer WORM guarantees solely from S3 API compatibility; use an Object-Lock-verified store/configuration for compliance retention. |
| **MinIO AIStor** | S3 WORM/versioned evidence | **Evaluate only if licensing/product fit and Object Lock requirement warrant it.** Object Lock requires versioning and retention creates delete markers rather than guaranteed immediate erasure.[69] |
| **ClickHouse** | Hybrid retrieval and audit analytics projection | **Adopt as rebuildable serving projection.** Never source of truth; route all access via gateway; model mutation/delete lag. |
| **Apicurio Registry** | Schema contracts | **Adopt.** Enforce transitive compatibility/validity in CI and runtime. |
| **OpenTelemetry Collector** | Trace/metric/log pipeline | **Adopt.** Collector centralizes retry, batching, encryption and sensitive-data filtering, but component maturity varies; allowlist stable components.[74] Propagate W3C Trace Context through broker headers so one ingestion can be traced end to end.[75] |
| **OpenFGA + OPA** | Fine-grained relation authorization + ABAC/policy | **Adopt if multi-tenant sharing/inheritance demands it.** Start with PostgreSQL RLS for catalog tables; introduce OpenFGA at gateway boundary. Pin FGA models and version policies; log decisions with redaction. |

## 8. Operational pitfalls / non-negotiables

1. **“Exactly once” marketing:** broker EOS is not end-to-end. Build idempotency ledgers and unique constraints at every sink; test kill/restart/replay scenarios.
2. **CDC slot disk exhaustion:** PostgreSQL replication slots retain WAL during Debezium outages; set a budget/alerts and incident runbook. `max_slot_wal_keep_size` bounds retention but risks an unusable/lost slot and subsequent resnapshot.[58][59]
3. **Outbox cleanup too early:** retain outbox long enough for CDC/recovery/reconciliation; cleanup only after a durable high-water/retention policy, never merely after an app publish ACK.
4. **Kafka compacted topics mistaken for audit:** compaction removes intermediate facts. Preserve a non-compacted immutable lifecycle/audit stream.
5. **Broker/schema migration:** a registry cannot make semantic breaking changes safe. Require dual read/write/backfill and contract tests.
6. **S3 deletion illusion:** versions/delete markers/WORM mean “not currently visible” differs from “physically erased.” Design legal hold/retention/encryption key destruction at policy level.[69]
7. **Search leakage:** tenant filters and ACL eligibility must be mandatory preconditions to retrieval; metadata/score/count side channels and cached answer leakage are real. Cache keys include tenant, principal/authorization version, policy version, and query intent.
8. **ACL change race:** ACL changes are lifecycle events. Gateway checks current eligibility synchronously from catalog/FGA before returning; asynchronous ClickHouse projection cannot be sole guard.
9. **ClickHouse delete/mutation lag:** remove retrieval eligibility first, then verify physical cleanup; do not promise immediate physical deletion from mutation submission.[91]
10. **Parser/model supply-chain and hostile documents:** malware scanning, decompression/PDF page bounds, network isolation, no macros, resource limits, and versioned reproducibility are required.
11. **Data quality invisible failures:** empty text, OCR noise, stripped tables, duplicate connector pages and clock skew must be observable quality states, not silently indexed.
12. **Re-embedding costs/backlog:** model/profile changes require capacity planning, generation aliases, canary evaluation, rollback, and old-index retirement tied to deletion/retention.
13. **Temporal history growth:** long workflows must Continue-As-New; activities must heartbeat and use stable idempotency keys.[65][92]
14. **Observability as a data leak:** telemetry/audit may contain document names, query text, user IDs and policy input. Redact/harden collector and OPA decision logs before centralizing.[74][8]
15. **Backup/DR is incomplete without restore tests:** exercise PostgreSQL catalog + broker topic + S3 evidence + schema registry + FGA tuple/model + secrets/KMS recovery as one consistent scenario. Measure RPO/RTO per plane.

## Decision path

**Recommended first production shape:** Kafka KRaft + Debezium Outbox + Apicurio; PostgreSQL catalog/RLS; SeaweedFS raw evidence with versioning and separately validated immutability/erasure policy; Temporal; Unstructured+Tika workers; ClickHouse hybrid projection; OpenTelemetry Collector; retrieval gateway; OpenFGA+OPA where resource sharing and ABAC exceed simple tenant isolation.

Choose Redpanda instead of Kafka only after accepting the BSL terms and verifying required Kafka Connect/Debezium/registry/backup operational behavior. Use NATS JetStream only for low-latency commands/ephemeral service coordination or as the primary broker for a deliberately smaller system that accepts its different ecosystem/semantics—not as a parallel duplicate of the organizational audit ledger.

## Sources

[4] https://cloudevents.io
[8] https://openpolicyagent.org/docs/management-decision-logs
[22] https://www.postgresql.org/docs/current/ddl-rowsecurity.html
[57] https://debezium.io/documentation/reference/stable/transformations/outbox-event-router.html
[58] https://debezium.io/documentation/reference/stable/connectors/postgresql.html
[59] https://www.postgresql.org/docs/current/logicaldecoding-explanation.html
[60] https://kafka.apache.org/41/design/design
[61] https://docs.redpanda.com/streaming/current/develop/transactions
[62] https://docs.redpanda.com/streaming/current/develop/produce-data/idempotent-producers
[63] https://docs.redpanda.com/streaming/current/get-started/licensing/overview
[64] https://docs.nats.io/reference/jetstream
[65] https://docs.temporal.io/activity-definition
[66] https://docs.temporal.io/self-hosted-guide/archival
[67] https://docs.unstructured.io/open-source/core-functionality/chunking
[68] https://tika.apache.org
[69] https://docs.min.io/aistor/administration/object-locking-and-immutability
[70] https://www.apicur.io/registry/docs/apicurio-registry/3.3.x/getting-started/assembly-rule-reference.html
[71] https://clickhouse.com/docs/reference/engines/table-engines/mergetree-family/annindexes
[72] https://clickhouse.com/docs/reference/engines/table-engines/mergetree-family/textindexes
[73] https://clickhouse.com/docs/concepts/features/security/access-rights
[74] https://opentelemetry.io/docs/collector
[75] https://opentelemetry.io/docs/concepts/context-propagation
[76] https://openfga.dev/docs/getting-started/immutable-models
[77] https://openpolicyagent.org/docs
[91] https://clickhouse.com/docs/concepts/features/operations/delete/overview
[92] https://docs.temporal.io/workflow-execution/limits
[93] https://docs.temporal.io/workflow-execution/continue-as-new
[94] https://openfga.dev/docs/getting-started/perform-list-objects
[95] https://kafka.apache.org/43/getting-started/upgrade
