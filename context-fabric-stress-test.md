# Context Fabric — Stress Test and Plugin-First Architecture

## Decision

Build **one deployable Context Fabric distribution from one monorepo**, but do **not** build one giant process or one shared, permissionless knowledge pool.

The platform should provide one canonical ingestion API, one retrieval API, one MCP adapter, one provenance/audit model, and one principal model for employees and agents. It should be deployable against an organization’s existing identity provider, authorization system, PostgreSQL, object storage, and event infrastructure.

The current XSAMA deployment may use Logto, but **Logto must be configuration, not a code dependency**. A generic OIDC provider adapter should work with Logto, Keycloak, Authentik, Entra ID, Okta, and compliant enterprise IdPs through discovery, JWKS validation, issuer/audience checks, and configurable claim mapping. OpenID Connect is an identity layer on OAuth 2.0, while SCIM is the standard HTTP protocol for cross-domain identity lifecycle management.[2][3]

## Critical corrections to the earlier proposal

1. **Tags are not authorization.** Tags classify and narrow a search; they never grant visibility. The gateway compiles mandatory tenant, context-space, purpose, classification, retention, and authorization filters before any lexical/vector candidate is hydrated. The caller-supplied tag query is only an additional `AND` constraint.
2. **An API key is a credential, not an identity or permission set.** It resolves to a platform principal such as `agent:...`, whose effective rights are evaluated at request time.
3. **Agents must not permanently clone their creator’s rights.** They receive a bounded, revocable delegation grant. An agent’s effective permission is the intersection of the creator’s current permission, the explicit agent ceiling, allowed purpose, resource/context-space selector, classification ceiling, and action policy.
4. **One repository must not mean one in-process plug-in runtime.** Untrusted source connectors and enrichers run outside the gateway in isolated workers. The gateway never dynamically imports arbitrary customer plug-in code.
5. **Raw evidence and LLM-derived knowledge are separate.** An LLM summary, inferred entity, tag, or relationship never overwrites a source message, official ticket, consent record, or CRM fact.
6. **No global total ordering promise.** Preserve ordering per source entity/conversation/document revision, use idempotency keys, and make consumers replay-safe. JetStream offers durable, replayable, at-least-once streams, so consumers must be idempotent.[4]

## The deployer-neutral target

```text
                         Existing IdP / directory
                  (OIDC, optional SCIM group sync)
                                 │
                                 ▼
┌────────────────────────────────────────────────────────────────────┐
│ Context Gateway                                                     │
│ REST/OpenAPI • MCP • authn adapter • delegation • policy compiler  │
│ audit • citations • redaction • context-packet assembly            │
└─────────────┬────────────────────┬───────────────────────┬─────────┘
              │                    │                       │
              ▼                    ▼                       ▼
       AuthZ provider         Context ledger          Plugin coordinator
   (OpenFGA/SpiceDB/OPA/      + object evidence         ├─ source connectors
    Cedar/local starter)      + outbox                  ├─ parsers
                                                         ├─ classifiers
                                                         ├─ indexers
                                                         └─ delivery adapters
              │                    │                       │
              └───────────┬────────┴────────────┬──────────┘
                          ▼                     ▼
                Event abstraction          Derived indexes
             (NATS first; Kafka later)    (pgvector first;
                                          Qdrant/OpenSearch later)
```

This follows zero-trust rather than network-trust logic: authentication and authorization are evaluated for the resource access, not inferred from where the request originated.[1]

## One monorepo, several deployable modules

```text
context-fabric/
├── contracts/
│   ├── openapi/                 # public REST contract
│   ├── asyncapi/                # event channels
│   ├── jsonschema/              # envelopes, records, plug-in manifests
│   └── authorization-fixtures/  # portable policy test cases
├── services/
│   ├── gateway/                 # HTTP API, authn, delegation, audit
│   ├── ingester/                # validation, quarantine, evidence write
│   ├── projector/               # entity, summary, index projections
│   ├── retriever/               # policy-first hybrid retrieval
│   ├── plugin-runner/           # isolated OCI/WASM execution host
│   └── mcp-adapter/             # thin MCP-to-gateway adapter
├── sdk/
│   ├── typescript/
│   ├── python/
│   └── go/
├── plugins/
│   ├── authn/generic-oidc/
│   ├── authz/openfga/
│   ├── authz/local-rbac/
│   ├── sources/chatwoot/
│   ├── sources/twenty/
│   ├── sources/mautic/
│   ├── sources/ses-events/
│   ├── parsers/docling/
│   ├── indexes/pgvector/
│   └── indexes/qdrant/
├── deploy/
│   ├── compose/                 # starter profile
│   ├── helm/                    # scaled profile
│   └── coolify/                 # XSAMA reference profile
└── docs/
    ├── adr/
    ├── plugin-authoring/
    ├── threat-model/
    └── operations/
```

OpenAPI provides a language-agnostic HTTP API description, while AsyncAPI is explicitly a contract between event senders and receivers. JSON Schema provides machine-validatable data structure and constraint definitions.[5][6][7]

## Context model: one platform, many governed context spaces

A single platform supports employee, customer, ticket, document, meeting, CRM, marketing, and agent context. It must not flatten those into one searchable bucket.

```text
organization
  └── context_space
        ├── domain             (support, sales, engineering, HR, finance)
        ├── resource           (case, account, project, document, person)
        ├── record             (message, fact, artifact, event, summary)
        └── derived projection (chunk, embedding, relationship, brief)
```

Every record carries system-enforced fields:

```json
{
  "organization_id": "org_...",
  "context_space_id": "case:SUP-412",
  "resource_type": "message",
  "resource_id": "msg_...",
  "classification": "confidential",
  "purpose_allowlist": ["support", "agent_assist"],
  "trust": "untrusted_external",
  "source_authority": "source_of_truth",
  "retention_policy_id": "customer-support-24m",
  "visibility_ref": "case:SUP-412#viewer",
  "tags": ["channel:email", "topic:billing", "brand:xsama"],
  "source_event_id": "evt_...",
  "content_hash": "sha256:..."
}
```

Use three tag classes:

| Class | Examples | May change access? |
|---|---|---:|
| **System scope** | `organization_id`, `context_space_id`, classification, retention, trust | No; gateway/policy controlled |
| **Controlled taxonomy** | domain, source, channel, topic, brand, case type | Only via authorized workflow |
| **Free/search tags** | analyst convenience labels | Never |

## Shared identity for employees and agents

```text
principal:user:<external-subject>
principal:agent:<uuid>
principal:service:<uuid>
principal:group:<external-or-synced-group>
```

All callers use the same gateway and the same authorization call. What differs is the authenticated principal and its delegation chain.

### Human request

```text
OIDC access token → generic OIDC adapter → principal:user:…
→ authorization provider + contextual policy → filtered context packet
```

### Agent request

```text
API key / M2M credential → agent principal
→ active delegation grant
→ current creator/owner relationship check
→ authorization provider + contextual policy
→ filtered context packet
```

Use an API key only to bootstrap a short-lived, audience-bound Context Fabric access token. Resource Indicators exist so OAuth clients can request a token specifically for its target resource, and DPoP can sender-constrain OAuth tokens to reduce replay misuse.[8][9]

### Delegation grant

```json
{
  "agent_principal": "agent:support-assist",
  "created_by": "user:alice",
  "owner": "team:support",
  "allowed_actions": ["context.search", "context.brief", "draft.reply"],
  "resource_selector": ["organization:acme", "team:support"],
  "purpose_allowlist": ["support", "agent_assist"],
  "classification_ceiling": "confidential",
  "send_policy": "human_approval_required",
  "expires_at": "...",
  "revoked_at": null
}
```

At request time:

```text
agent_effective_access =
  agent_baseline
  ∩ active_delegation_grant
  ∩ creator/owner_current_authority
  ∩ resource_relationship_authorization
  ∩ contextual_policy
```

Consequences:

- A manager leaving a team, losing authority, or revoking an agent immediately narrows or disables agent access.
- An employee cannot create an agent with more rights than they presently hold.
- A scheduled agent must have an explicit persistent delegation grant; it cannot impersonate a person indefinitely.
- High-risk write/send actions remain separate permissions with approval requirements.
- Every result and action audit record includes `principal`, `actor_chain`, `delegation_grant_id`, `policy_revision`, and `authorization decision`.

OAuth Token Exchange standardizes delegation and impersonation semantics, including representing an actor in an issued token; use it where the connected IdP supports it, but keep the Context Fabric delegation model independent of any single provider.[10]

## Pluggable control plane

### Provider interfaces

```text
IdentityProvider
  authenticate(credentials) -> AuthenticatedPrincipal
  discover() -> OIDC/JWKS metadata
  map_claims() -> normalized subject, groups, organization attributes

DirectoryProvider
  sync_users_groups() -> normalized directory changes

AuthorizationProvider
  check(subject, action, resource, context) -> decision
  batch_check(...)
  resolve_candidate_scope(...) -> safe prefilter, if supported

PolicyProvider
  evaluate(subject, action, record_metadata, purpose) ->
    allow/deny + redaction profile + obligations

CredentialProvider
  create/revoke/rotate agent credentials
  resolve_key() -> agent principal only

EventBus, EvidenceStore, IndexProvider, ParserProvider, EmbeddingProvider
  are independently swappable infrastructure adapters.
```

The first-party distribution should ship a **generic OIDC adapter**, a **local development identity adapter**, and an **OpenFGA authorization adapter**. OpenFGA models authorization as relationship checks against versioned models and tuples; SpiceDB is a viable interchangeable relationship-based alternative.[11][12]

OPA or Cedar can handle conditional/attribute policy when needed, but do not deploy two policy engines in the first production slice.[13][14]

For policy portability, run a required authorization fixture suite against every provider adapter. A policy must produce identical allow/deny and redaction obligations for shared test cases before it can be selected as production provider.

## Plug-in model: extensible without letting extensions own the core

### Trust tiers

| Tier | Examples | Execution model | Direct database access? |
|---|---|---|---:|
| Core trusted | gateway, ledger, authorization adapter | reviewed core process | Limited, service-specific |
| First-party worker | Chatwoot/Twenty/Mautic connector | isolated container/job | No |
| Third-party plug-in | customer source/parser/enricher | signed OCI worker or sandboxed WASM | No |
| Deterministic transform | validation/redaction/tag transform | WASM with explicit host capabilities | No |

Use OCI image digests for networked connectors. Use WASM/WASI for compact deterministic transforms where its capability boundary is useful. The WebAssembly Component Model is designed for interoperable components, and Wasmtime provides configurable resource controls; neither removes the need to restrict host capabilities and outbound network access.[15][16]

Each plug-in must supply a manifest:

```yaml
id: com.example.slack-source
api_version: context-fabric.plugin/v1
kind: source
runtime: oci | wasm
image_or_module_digest: sha256:...
config_schema_ref: registry://plugins/slack-source/1.2.0
capabilities:
  - source.read:slack
  - network.egress:slack.com
  - event.publish:context.ingest.request.v1
secrets:
  - slack.oauth-token
emits:
  - context.ingest.request.v1
```

Rules:

- A plug-in receives only source-specific credentials through a secret reference.
- It can submit validated intake events, but cannot directly write canonical facts, alter authorization tuples, query arbitrary context, or call an agent action endpoint.
- Plug-in packages are pinned by immutable digest, scanned, SBOM-attested, signature-verified, and approved before activation. SLSA and Sigstore provide relevant supply-chain integrity/provenance controls.[17][18]
- Disable outbound network by default; grant exact destination allowlists per connector.

## Standard ingestion contract

Use **CloudEvents 1.x** as the outer event envelope and versioned JSON Schema payloads registered in Git initially; promote to Apicurio Registry when multiple teams/connectors need runtime governance. CloudEvents standardizes event descriptions, while Apicurio can store JSON Schema, OpenAPI, AsyncAPI, and enforce validity/compatibility/integrity rules for evolving artifacts.[19][20][21]

```text
Source connector
  → signed source event
  → validate schema + signature + tenant binding
  → quarantine/scan/size-limit/rate-limit
  → immutable evidence object + SHA-256
  → canonical append-only event + transactional outbox
  → durable event stream
  → async projection workers
  → entities, tags, embeddings, summaries, index points
```

Mandatory ingestion controls:

```text
UNIQUE(source_system, source_external_id, source_revision)
UNIQUE(idempotency_key)
per-resource source sequence / revision check
content hash
append-only evidence/event records
tombstone and redaction events; no destructive overwrite
DLQ + replay cursor + reconciliation/backfill
source health and completeness status
```

The transactional outbox pattern avoids a service committing data but crashing before publishing its event; it still requires idempotent consumers because the relay can publish more than once. Debezium supports a purpose-built outbox router if CDC is later needed.[22][23]

### Evidence vs. derived context

| Layer | Authority | Mutation rule |
|---|---|---|
| Evidence | Original source message/file/API response | Immutable; tombstone/redact by new event |
| Canonical event/fact | Source-authoritative change or approved business record | Versioned/source-aware |
| Derived context | tags, extracted entities, summaries, graph facts | Rebuildable, confidence/review state required |
| Search indexes | vectors, lexical index, cache | Fully disposable/rebuildable |

Derived objects must retain source event IDs, model/parser version, prompt/template version where applicable, confidence, reviewer, and timestamp. Graphiti is reasonable later as a derived temporal graph projection, not the canonical ledger or authorization system.[24]

## Standard retrieval contract

The gateway owns all retrieval. Agents, employees, MCP clients, and internal applications never directly query PostgreSQL, object storage, Qdrant, OpenSearch, or a source platform.

```text
authenticate principal
→ resolve delegation/organization/purpose
→ authorize context-space/resource access
→ compile non-bypassable mandatory metadata predicate
→ candidate search within that predicate only
→ exact batch authorization before content hydration
→ apply field/content redaction obligations
→ assemble cited context packet
→ immutable audit event
```

A request has a fixed schema:

```json
{
  "query": "What commitments were made in the active case?",
  "purpose": "support",
  "scope": {"case_id": "SUP-412"},
  "filters": {
    "include_tags": ["topic:billing"],
    "time_range": {"from": "...", "to": "..."}
  },
  "max_items": 12,
  "token_budget": 3000
}
```

The server-generated filter is logically:

```text
organization_id = caller.organization
AND context_space ∈ authorized_context_spaces
AND classification ≤ caller/delegation ceiling
AND purpose ∈ record.purpose_allowlist
AND retention_status = active
AND requested_tags ⊆ record.tags
```

For vector search, the index may receive only a minimal stable record ID plus mandatory partition metadata. It returns candidate IDs, not plaintext; the gateway re-authorizes before hydrating text. Qdrant supports payload filtering and tenant-oriented storage, but that is an index optimization—not the authorization authority.[25]

For every response return:

```text
source citations
record versions and timestamps
policy/authz revision
redactions applied
actor/delegation identity
confidence/review state for derived facts
```

MCP is a delivery adapter over this same API, not a second data plane. The current MCP authorization specification treats protected MCP servers as OAuth resource servers and requires protected-resource metadata discovery for HTTP transports.[26]

## Stress-test findings

| Failure scenario | If built naively | Required control | Acceptance test |
|---|---|---|---|
| Tag changed from `restricted` to `public` | Unauthorized search results | Classification/visibility are system policy fields; tag change cannot grant access | Attempt downgrade as ordinary employee and agent; expect deny + audit |
| Manager creates broad autonomous agent | Permanent privilege clone | Delegation intersection, TTL, creator-current-authority check, action caps | Remove manager role; next agent request must fail/lose access |
| API key leaks | Attacker gets a human’s full access | Key maps to an agent principal; exchange for short-lived audience-bound token; rotate/revoke | Revoke key and ensure gateway, cache, MCP all deny within target SLA |
| Customer message contains prompt injection | Agent follows data as instructions | Trust labels, quoted external evidence, separate tools, least privilege, human approval | Corpus of direct/indirect injection fixtures cannot read unrelated context or send |
| Webhook retried/out of order | Duplicate timeline or stale state overwrites newer data | Idempotency/revision keys, per-resource ordering, replay-safe projections | Replay same event N times and reversed revisions; final ledger/index hash must match |
| Connector is compromised | Connector exfiltrates or corrupts context | Isolated runner, source-limited secret, no direct DB, egress allowlist, signed digest | Plug-in attempts DB/network access outside manifest; expect blocked |
| Policy provider is slow/down | Fail-open retrieval or universal outage | Fail closed for sensitive reads/actions; bounded decision cache keyed by policy revision; degraded-mode contract | Kill provider; verify no stale broadened data is returned |
| Authorization sync lags | Former employee sees records | Short cache TTL, policy revision invalidation, critical exact checks | Revoke user; verify sensitive access denied at documented revocation SLO |
| Data deletion request | Raw record gone but summaries/vectors/caches remain | Tombstone cascade and deletion workflow across evidence, projections, index, cache, backups | Delete fixture then prove search/brief/index rebuild cannot recover it |
| Huge tenant / hot case | One partition/index becomes bottleneck | Partition by organization/context-space; sharded workers; capacity metrics | Load test hot tenant and normal tenants; preserve isolation and SLOs |
| Schema change breaks workers | Silent corruption or lost fields | Schema registry, compatibility gate, consumer contract tests | Reject incompatible producer schema before deployment |
| DB admin alters history | “Immutable” audit claim is false | Tamper-evident hashes + signed checkpoints + separate backup/retention controls | Modify historical fixture; integrity verifier must detect mismatch |

Prompt injection and excessive agency are central risks for a context system: OWASP explicitly recommends least privilege, separating untrusted external content, validating output, and human approval for high-risk actions.[27]

## Do not overbuild on day one

### Build now

```text
1. Core gateway + canonical event/record contracts
2. Generic OIDC/JWT adapter and local development identity adapter
3. One authorization provider adapter: OpenFGA
4. Agent principal + bounded delegation grants + key lifecycle
5. PostgreSQL canonical ledger + pgvector, S3-compatible evidence store
6. NATS JetStream event adapter and outbox/replay implementation
7. One source connector end-to-end: Chatwoot
8. One retrieval API + cited context packets + MCP adapter
9. Audit, redaction, deletion, and authorization test harness
```

### Keep optional behind interfaces

```text
SCIM directory synchronization
SpiceDB, OPA, Cedar authorization/policy adapters
Qdrant/OpenSearch indexes
Docling/unstructured parsing adapters
Graphiti temporal graph projection
Kafka/Redpanda event-bus adapter
Temporal workflow orchestration
OpenMetadata/DataHub integration/catalog projection
```

OpenMetadata and DataHub offer useful connector, lineage, source/sink, and governance patterns, but they are data catalogs rather than a secure cross-channel conversational context authority. Reuse their lessons or integrate them later; do not make either the customer/employee context ledger.[28][29][30]

Airbyte, NiFi, and Redpanda Connect can accelerate some batch/stream connectors, but should be optional edge ingestion tools rather than the place where context authorization, canonical semantics, or agent retrieval is enforced.[31][32][33]

## Recommended deployment profiles

### Starter profile: portable, small organization

```text
Existing OIDC provider
PostgreSQL + pgvector
S3-compatible evidence store
NATS JetStream
Context Gateway + workers
OpenFGA
```

### XSAMA reference profile

```text
Coolify application VM:
  Context Gateway, ingester, projector, retriever, MCP adapter,
  OpenFGA/OPA only as private services

Dedicated data LXCs:
  PostgreSQL LXC 103: context_platform database and authorization data
  SeaweedFS LXC 108: context-raw, context-derived, context-quarantine buckets
  New dedicated NATS service: durable event plane

Identity:
  Generic OIDC configuration pointed at current Logto instance
  No Logto-specific source imports or schemas in core
```

Do not expose PostgreSQL, object storage, NATS, OpenFGA, or derived indexes publicly. Only the gateway/MCP endpoint should be deliberately reachable, with normal OAuth resource-server protections.

## Non-negotiable release gates

1. **Authorization matrix**: employee, manager, owner, assigned support member, external customer, agent, revoked agent, and platform admin test fixtures.
2. **Delegation regression suite**: agents cannot exceed, outlive, or retain the creator’s authority after revocation.
3. **Tenant/cross-context fuzzing**: caller-controlled tags, IDs, pagination, filters, and vector query parameters cannot widen scope.
4. **Replay test**: duplicate, reordered, and delayed ingestion produces identical canonical projections.
5. **Deletion test**: prove deletion/redaction across raw evidence references, chunks, vectors, summaries, caches, graph projection, and exports.
6. **Prompt-injection test**: malicious external content cannot alter authorization, policy, tools, or outward actions.
7. **Plug-in containment test**: a plug-in cannot use undeclared egress, secrets, file access, or APIs.
8. **Schema compatibility test**: breaking payload changes fail CI/registry admission.
9. **Load/SLO test**: bounded latency and no cross-tenant leak under concurrent retrieval and ingestion.
10. **Restore drill**: recover ledger, evidence mappings, policy revision, and indexes from backup, then verify integrity.

## Final recommendation

Proceed with a product-shaped **Context Fabric monorepo** whose hard boundaries are:

```text
one API surface
one canonical context contract
one principal/delegation model
one policy-first retrieval path
one audit/provenance model
many replaceable adapters and isolated plug-ins
```

Use the existing Logto deployment through the generic OIDC configuration today, but treat all identity, authorization, storage, eventing, indexing, and parsing components as provider plugins. Employees and agents should share the same infrastructure, but agents must operate as independent principals with bounded delegated authority—not as copies of the people who created them.

## Independent-review reconciliation

Three independent reviews reinforced the core design, but require several important adjustments before this becomes a build specification.

### 1. Ship deployment profiles, not one mandatory infrastructure stack

The product contract remains provider-neutral. The reference deployment must not force every organization into the same broker or index.

| Profile | Eventing | Retrieval projection | Appropriate use |
|---|---|---|---|
| **Starter / homelab** | NATS JetStream | PostgreSQL + pgvector | One organization, low operational overhead, modest corpus, limited connectors |
| **Enterprise replay / CDC** | Apache Kafka + Debezium outbox | OpenSearch hybrid search | Long retention, high-volume CDC, broad connector ecosystem, many independent consumers |
| **Vector-specialist** | Either bus adapter | Qdrant plus lexical search provider | Dense retrieval/filtering is the bottleneck and a separate vector tier is justified |

For XSAMA's first controlled implementation, retain the starter profile: NATS JetStream and pgvector, with a strict event-bus/index adapter boundary. Do **not** make NATS or pgvector a product assumption. JetStream supplies durable streams, replay and at-least-once consumption; Kafka is the stronger default for deployments whose primary requirements are long replay, CDC, and a large connector ecosystem.[4][40]

The authoritative ledger stays PostgreSQL and raw evidence stays S3-compatible object storage in every profile. Search/vector systems are rebuildable projections only. OpenSearch becomes the reference scaled retrieval adapter because it combines lexical, semantic, and hybrid ranking; Qdrant remains a payload-filtered vector adapter, not an authorization authority.[25][28]

### 2. The canonical "creator" model is insufficient by itself

Replace “creator owns agent access” with an explicit **delegation subject, actor, owner, and runtime** model:

```text
subject          = human or service whose current authority bounds the work
actor            = the agent principal executing the work
owner/steward    = team or organization responsible for the agent lifecycle
runtime          = separately authenticated worker/container identity
delegation grant = resource/action/purpose/expiry/budget constraint
```

The gateway evaluates all of those on every sensitive retrieval or action. The agent must never receive a copied human token, use the creator's `sub`, or silently gain future creator permissions. OAuth BCP 240 emphasizes exact redirect matching, least privilege, audience restriction, and modern token protections; MCP security guidance forbids token passthrough across resources because it enables confused-deputy failures.[34][35]

For employee browser sessions, use OIDC authorization code + PKCE. Normalize humans using immutable `issuer + subject`, not email. Use SCIM only to synchronize lifecycle/users/groups; it is not the live authorization decision engine.[2][3]

### 3. Add a consistency contract to the authorization-provider port

The `AuthorizationProvider` adapter needs an explicit request-consistency option:

```text
min_latency       # ordinary low-risk discovery
at_least_as_fresh # after a grant/revocation/share change
fully_consistent  # sensitive read, send, delete, or policy mutation
```

OpenFGA supports higher-consistency query modes, while SpiceDB exposes ZedTokens to request at-least-as-fresh or exact-snapshot authorization views. This makes either provider viable behind the same port and prevents a just-revoked employee or agent from receiving a stale allow result.[36][37]

**Default product adapter:** OpenFGA first, because it is an approachable deployer-neutral ReBAC implementation. **Promotion path:** select SpiceDB when the deployment needs caveats, expiring relationships, and explicit read-after-write consistency as first-class operational requirements. Core contracts must mention neither product's schema language.

### 4. Strengthen the ingestion and deletion state machine

The minimum canonical state machine is:

```text
RECEIVED → VALIDATED → QUARANTINED | ACCEPTED
ACCEPTED → PROJECTING → INDEXED | FAILED
INDEXED → RECLASSIFIED | TOMBSTONED
TOMBSTONED → PURGE_PENDING → PURGED
```

Every transition is append-only and versioned. A tombstone must dominate any older upsert, including after replay or restoration. Each canonical revision stores a derivative manifest linking its parsed text, chunks, embeddings, summaries, graph facts, caches, exports, and object-store versions. A restore procedure must replay current tombstones/revocations before making restored data queryable. NIST's media-sanitization guidance supports treating deletion as a measured lifecycle rather than merely removing a single database row.[38]

Use JSON Schema payload compatibility as the enforceable event gate; AsyncAPI documents channels and CloudEvents standardizes the envelope, but neither independently guarantees safe domain-schema evolution. Apicurio Registry supports validity, compatibility, and integrity rules for the schema/API artifacts it stores.[19][20][21]

### 5. Add parser and telemetry hardening to the first release gates

Adopt **Docling** as the primary rich-document parser and **Apache Tika** as broad-format fallback behind a `ParserProvider` port. Every parser worker needs resource ceilings for archive depth, decompression size, file size, page count, CPU, memory, wall time, and network egress. Low-confidence conversions must be quarantined or routed to review rather than automatically becoming agent-ready context.

Propagate OpenTelemetry/W3C trace context for operations, but never treat `traceparent`, `tracestate`, baggage, CloudEvents extensions, or plugin-provided fields as identity/authorization inputs. Scrub prompts, PII, credentials, session tokens, and source content from telemetry by default; OpenTelemetry explicitly advises data minimization and provides filtering/redaction processors for sensitive attributes.[39]

### 6. Updated hard no-go criteria

Do not ingest production-wide organizational history or enable autonomous agent actions until the following are demonstrated in a controlled environment:

```text
• agent revocation propagates through cached/replayed requests within a measured SLO
• delete/tombstone propagation prevents recovery from every derivative and restored backup
• duplicate and reordered events cannot resurrect deleted/reclassified content
• tag changes cannot expand access without an independent authorized ACL/policy change
• an untrusted source document/plugin manifest cannot influence policy or tool execution
• every MCP/downstream token is resource-audience-bound and never passed through
• one noisy tenant cannot breach another tenant's latency, quota, or disclosure boundaries
• a complete disclosure decision can be reconstructed without logging plaintext sensitive context
```

## Sources

[1] https://csrc.nist.gov/pubs/sp/800/207/final
[2] https://openid.net/specs/openid-connect-core-1_0.html
[3] https://datatracker.ietf.org/doc/html/rfc7644
[4] https://docs.nats.io/nats-concepts/jetstream
[5] https://www.openapis.org
[6] https://json-schema.org/overview/what-is-jsonschema
[7] https://www.asyncapi.com/docs/concepts/asyncapi-document
[8] https://datatracker.ietf.org/doc/html/rfc8707
[9] https://datatracker.ietf.org/doc/html/rfc9449
[10] https://datatracker.ietf.org/doc/html/rfc8693
[11] https://openfga.dev/docs/concepts
[12] https://authzed.com/docs/spicedb/concepts/relationships
[13] https://www.openpolicyagent.org/docs/latest
[14] https://www.cedarpolicy.com/en
[15] https://component-model.bytecodealliance.org
[16] https://wasmtime.dev
[17] https://slsa.dev
[18] https://www.sigstore.dev
[19] https://cloudevents.io
[20] https://www.apicur.io/registry/docs/apicurio-registry/3.3.x/getting-started/assembly-intro-to-the-registry.html
[21] https://www.apicur.io/registry/docs/apicurio-registry/3.3.x/getting-started/assembly-rule-reference.html
[22] https://microservices.io/patterns/data/transactional-outbox.html
[23] https://debezium.io/documentation/reference/stable/transformations/outbox-event-router.html
[24] https://github.com/getzep/graphiti
[25] https://qdrant.tech/documentation/guides/multiple-partitions
[26] https://modelcontextprotocol.io/specification/latest/basic/authorization
[27] https://genai.owasp.org/llmrisk/llm01-prompt-injection
[28] https://docs.open-metadata.org/v1.12.x/developers/contribute/codebase-deep-dives/metadata-ingestion
[29] https://docs.datahub.com/docs/metadata-ingestion
[30] https://docs.datahub.com/docs/authorization/policies
[31] https://docs.airbyte.com/platform/connector-development
[32] https://nifi.apache.org/docs/nifi-docs/html/developer-guide.html
[33] https://docs.redpanda.com/redpanda-connect/components/inputs/about
[34] https://www.rfc-editor.org/info/rfc9700
[35] https://modelcontextprotocol.io/docs/2026-07-28/tutorials/security/security_best_practices
[36] https://authzed.com/docs/spicedb/concepts/consistency
[37] https://openfga.dev/docs/interacting/consistency
[38] https://csrc.nist.gov/pubs/sp/800/88/r2/final
[39] https://opentelemetry.io/docs/security/handling-sensitive-data
[40] https://kafka.apache.org/documentation
