# XSAMA Context Fabric — strategic architecture recommendation

**Status:** research-backed architecture decision; no deployment or infrastructure change performed.

## Decision

Build an organization-owned **Context Gateway** and **Context Catalog** as the only data/API/MCP boundary. Do not adopt Hindsight, Mem0, a vector database, a graph database, or an agent framework as the organization-wide system of record or access-control system.

The system must preserve a strict separation:

```text
raw evidence → immutable source/revision record → derived artifacts → indexes
```

Only the first two are authoritative. Extracted text, chunks, embeddings, entities, summaries, graph facts, and agent memories are disposable, versioned projections.

## Recommended first production shape

```text
Logto
  └─ human and M2M authentication, organization context, coarse scopes

Context Gateway
  ├─ versioned REST API (canonical interface)
  ├─ thin remote MCP adapter (same authorization path)
  ├─ audit emitter
  └─ authorization-aware retrieval and ingestion admission

OpenFGA + OPA
  ├─ OpenFGA: relation/resource permissions
  └─ OPA: classification, purpose, redaction, approval and agent ceilings

PostgreSQL 18 + pgvector
  ├─ canonical resource/revision/artifact/provenance catalog
  ├─ initial hybrid/vector retrieval
  ├─ RLS tenant-containment backstop
  └─ processed-event/idempotency ledger

SeaweedFS S3
  ├─ quarantined source material
  ├─ approved raw evidence
  └─ extraction/redaction artifacts

Kafka KRaft + Debezium Outbox + Apicurio Registry
  ├─ durable replayable organizational event ledger
  ├─ schema-governed data contracts
  └─ asynchronous context processing

Temporal workers
  └─ scan → extract → classify → redact → chunk → embed → index,
     plus deletion/redaction workflows
```

Use the existing ClickHouse cluster for high-volume audit, observability, timeline analytics, and later retrieval experiments. It is never canonical truth. Start semantic retrieval in PostgreSQL/pgvector; add Qdrant only when measurements demonstrate a dedicated vector tier is required. Qdrant supports payload-driven multitenancy and sharding, but its filters are not a document-level authorization system.[2]

## Why Kafka rather than NATS as the primary ledger

For an organization-wide system that must ingest all context categories, replay history, support CDC/outbox connectors, and govern schemas over time, make **Kafka KRaft** the primary durable event plane. Kafka transactions improve correctness within the log-processing boundary, but do not create end-to-end exactly-once effects across object storage, OCR, embeddings, databases, and external systems.[60]

Use **NATS JetStream** later only for low-latency internal commands or transient signaling if there is a concrete need. Its at-least-once semantics and narrower CDC/schema ecosystem make it a less appropriate primary organizational ledger.[64]

## Standard contracts

### 1. Event envelope

All ingestion is CloudEvents 1.x with a versioned, schema-registry-governed payload. CloudEvents standardizes event description and delivery, while AsyncAPI describes the asynchronous interface contract.[4][5]

Required fields:

```text
event_id, event_type/version, source, producer, tenant_id, brand_id,
resource_id, revision_id, occurred_at, observed_at, classification,
content-trust, source-authority, schema version, correlation/causation ID,
trace context, immutable artifact pointer and SHA-256 where content exists
```

Use topic families rather than a topic per tenant:

```text
context.lifecycle.v1
context.work.v1
context.derived.v1
context.audit.v1
context.dlq.v1
```

Partition lifecycle events by `(tenant_id, resource_id)` so one resource's state transitions remain ordered. Define retention per topic; compacted current-state topics never replace the non-compacted lifecycle/audit history.

### 2. Canonical resource model

```text
Organization / tenant
Brand
Principal / team / service identity
Logical resource
Observed revision
Evidence artifact
Derived artifact
Chunk
Embedding generation
Context event
Access relationship
Policy decision
Audit event
```

Every derived record must include:

```text
source_event_id, resource_id, revision_id, artifact hash,
extractor/chunk/embedder/model version, policy/redaction version,
FGA object reference, confidence, review state, superseded/deleted state
```

### 3. Tags are classification, not authorization

Controlled tags include:

```text
domain: customer-support | sales | finance | engineering | HR
classification: public | internal | confidential | restricted
trust: trusted-system | trusted-internal | untrusted-external | generated
authority: source-of-truth | corroborating | user-claim | inferred
purpose: support | account-management | marketing | finance | agent-assist
retention: 30d | 24m | legal-hold | indefinite-policy
```

Tags guide policy, retrieval, retention, and redaction. They never grant access by themselves.

## Standard ingestion path

```text
source webhook/API/upload
→ authenticate and validate envelope/schema
→ idempotency check
→ quarantine + malware / archive / MIME validation
→ protected raw evidence store
→ PostgreSQL catalog + outbox in one transaction
→ Debezium publishes durable event
→ Temporal workflow enriches/derives projections
→ retrieval eligibility only after provenance + ACL checks
```

Use a transactional outbox. It prevents the split-brain failure where source state is committed but its event is not published. Debezium's Outbox Event Router is designed to capture outbox records and preserve a unique event ID for duplicate removal.[57]

Design for **at-least-once transport plus idempotent effects**. Every materializer owns a durable `(consumer_name, event_id)` receipt or equivalent unique projection key. Never claim full end-to-end exactly once.

Use Tika as a sandboxed baseline format/text detector and Unstructured behind an extractor interface for structure-aware partitioning. Unstructured's partition/chunk pipeline supports common enterprise file types; pin parser/chunk profile versions and retain element/page provenance.[81]

Protect the intake with ClamAV before parsing attachments and use Presidio or policy-specific detectors for PII/secret classification and safe retrieval derivatives.[89][88]

## Standard retrieval path

```text
validate Logto token and resource audience
→ derive tenant/brand from token, never request body
→ scope gate
→ OpenFGA relationship authorization
→ OPA disclosure/redaction/purpose decision
→ PostgreSQL RLS transaction context
→ mandatory tenant/classification/ACL-filtered hybrid retrieval
→ final FGA/state recheck
→ redacted cited context packet
→ immutable audit event
```

Logto organization API resources and organization M2M roles fit separate users, managers, and agents operating under a tenant-aware API.[11]

OpenFGA decides who may access which resources through an authorization model and relationship tuples.[78] OPA is reserved for contextual policy rather than duplicated relationship rules.[23] PostgreSQL RLS is a defense-in-depth tenant boundary; runtime database roles must be non-owner and non-`BYPASSRLS`.[22]

Return context only as a bounded packet:

```text
summary
facts
relevant timeline entries
stakeholders
citations with resource/revision/locator
applied redactions
policy version and request audit ID
agent action restrictions
```

Do not return raw database rows, unrestricted document bodies, opaque vector hits without provenance, or authorization metadata about denied resources.

## API and MCP

The REST API is canonical. MCP is a read-only adapter over the same gateway, not a second store or a shortcut around policy.

Initial MCP tools:

```text
context.search
context.get
context.brief_case
context.brief_person
context.request_access
```

Remote MCP should use OAuth resource-server semantics, validate tokens intended for the gateway/MCP audience, and never pass client tokens through to downstream systems.[79]

Each agent receives a distinct Logto M2M identity. Its effective privilege is:

```text
user rights ∩ agent capability ceiling ∩ task/session grant ∩ requested scope
```

Start every agent read-only. Allow it to draft only after policy/evaluation tests pass. Sending, sharing, deletion, or policy modifications require dedicated scope and an approval workflow.

## Scale and specialization rules

| Need | Component decision |
|---|---|
| Initial relational + semantic retrieval | PostgreSQL + pgvector |
| Dedicated vector scale-out | Qdrant, private behind gateway |
| Evolving customer/case relationship queries | Graphiti as a derived temporal graph projection, not truth source |
| Long-running ingestion/deletion orchestration | Temporal |
| Organization-wide data/AI governance catalog | OpenMetadata later, not runtime retrieval authority |
| Per-agent personal preferences | Optional isolated memory component, never shared truth |
| Retrieval pipeline implementation | Haystack preferred; LlamaIndex is an alternative connector layer |

Graphiti is useful for temporal facts, provenance episodes, incremental graph updates, and hybrid retrieval, but it adds a graph database and does not provide this platform's authorization model.[1]

## Anti-corruption and deletion rules

```text
- Never allow an LLM summary or inferred fact to overwrite source evidence.
- Keep raw evidence, canonical source metadata, derivations, and indexes separate.
- Rebuild indexes from retained lifecycle events plus evidence.
- Store source/revision/hash/model/profile lineage for every derived result.
- Treat external content as untrusted data, never as executable instructions.
- Revoke gateway visibility before asynchronous physical deletion begins.
- Delete/redact all derived artifacts and indexes through a provenance-driven saga.
- Verify zero retrievable results before marking deletion complete.
- Make immutable/WORM retention and erasure legal-policy decisions explicit; a deleted S3 key does not prove old versions were physically erased.
```

## Repository/package shape

Create one versioned monorepo, for example `xsama-context-fabric`:

```text
contracts/
  cloudevents/          # JSON/Avro/Protobuf schemas
  asyncapi/             # broker contracts
  openapi/              # REST contract
  taxonomy/             # classifications, tags, retention matrix

services/
  context-gateway/
  context-ingest/
  context-retrieval/
  context-audit/

workers/
  connector-chatwoot/
  connector-twenty/
  connector-mautic-ses/
  extraction/
  classification/
  redaction/
  embedding/
  deletion/

authorization/
  openfga/              # model, fixtures, migration versions
  opa/                  # policy, tests, signed bundle pipeline

infra/
  kafka/
  debezium/
  apicurio/
  temporal/
  observability/

evals/
  authorization/
  retrieval/
  prompt-injection/
  deletion-replay/
```

## Deployment sequence

1. Write the event, REST, taxonomy, OpenFGA, and OPA contracts first.
2. Build a synthetic-data gateway proof: employee, manager, agent, customer case, internal restricted note.
3. Prove cross-tenant denial, revocation, RLS containment, cache invalidation, and audit completeness.
4. Deploy the durable event and workflow plane.
5. Connect Chatwoot, Twenty, Mautic/SES as the first real sources.
6. Enable read-only REST retrieval; validate citations, retention, and response quality.
7. Add remote MCP only after REST follows the policy path correctly.
8. Scale retrieval/index infrastructure only from measured p95 latency, corpus size, ingest backlog, and recall data.

## Required test gates

```text
- duplicate and replay events do not duplicate timeline/projection entries
- resource revision preserves prior evidence and temporal validity
- manager cannot see restricted HR/legal/security data by role alone
- employee cannot retrieve manager-only content via vector similarity, cache,
  pagination, summaries, citations, score/count side channels, or MCP
- consent/suppression changes are visible to marketing policy before send
- deletion prevents all future retrieval before physical purge finishes
- untrusted text cannot elevate privileges, alter policy, or invoke a tool
- audit contains actor/client/policy/FGA model/retrieval counts but no secret/raw content
```

## Evaluation and observability

Use the existing Langfuse installation when its OIDC flow is repaired, alongside OpenTelemetry. Record only redacted metadata by default; OpenTelemetry provides filter/redaction processors for sensitive attributes.[90]

Maintain versioned retrieval and access-control test datasets. Ragas can supply systematic RAG evaluation metrics, but security tests and human review are separate mandatory gates.[10]

## Conclusion

The Context Fabric is a **custom, standards-led control plane with replaceable specialist components**. It centralizes the only things that should be unique to XSAMA: organizational policy, tenancy, identity, provenance, retention, context contracts, and safe agent access. It avoids betting the organization on a single memory/RAG product while still benefiting from mature OSS infrastructure.

## Sources

[1] https://github.com/getzep/graphiti
[2] https://qdrant.tech/documentation/manage-data/multitenancy
[4] https://cloudevents.io
[5] https://www.asyncapi.com/docs/concepts/asyncapi-document
[10] https://docs.ragas.io/en/stable
[11] https://docs.logto.io/authorization/organization-level-api-resources
[22] https://www.postgresql.org/docs/current/ddl-rowsecurity.html
[23] https://www.openpolicyagent.org/docs/integration
[57] https://debezium.io/documentation/reference/stable/transformations/outbox-event-router.html
[60] https://kafka.apache.org/41/design/design
[64] https://docs.nats.io/reference/jetstream
[78] https://openfga.dev/docs/concepts
[79] https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization
[81] https://docs.unstructured.io/open-source/core-functionality/partitioning
[88] https://github.com/data-privacy-stack/presidio
[89] https://docs.clamav.net
[90] https://opentelemetry.io/docs/security/handling-sensitive-data
