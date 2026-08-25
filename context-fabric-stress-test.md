# Context Fabric — Historical Reference Architecture

**Status:** Historical. Kept for background and rationale. Active decisions live in
[`docs/adr/`](docs/adr/README.md); when this document conflicts with an ADR, the ADR
wins (service topology, consistency vocabulary, OPA in first slice, Chatwoot-special
intake, and related topics). Profile-specific choices remain adapter-level.

**Scope:** A self-hosted, multi-tenant organizational context platform for customer
and internal sources, durable ingestion, provenance, governed employee/agent
retrieval over REST and MCP, redaction, deletion, and audit. It is an architecture
decision, not a claim that deployment has occurred.

## Decision

Build **one deployable Context Fabric distribution from one monorepo**, but do **not** build one giant process or one shared, permissionless knowledge pool.

The platform should provide one canonical ingestion API, one retrieval API, one MCP adapter, one provenance/audit model, and one principal model for employees and agents. It should be deployable against an organization’s existing identity provider, authorization system, PostgreSQL, object storage, and event infrastructure.

The current XSAMA deployment may use Logto, but **Logto must be configuration, not a code dependency**. A generic OIDC provider adapter should work with Logto, Keycloak, Authentik, Entra ID, Okta, and compliant enterprise IdPs through discovery, JWKS validation, issuer/audience checks, and configurable claim mapping. OpenID Connect is an identity layer on OAuth 2.0, while SCIM is the standard HTTP protocol for cross-domain identity lifecycle management.[2][3]

## Architecture invariants

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

The initial controlled vocabulary is:

```text
domain: support | sales | finance | engineering | HR
classification: public | internal | confidential | restricted
trust: trusted_system | trusted_internal | untrusted_external | generated
authority: source_of_truth | corroborating | user_claim | inferred
purpose: support | account_management | marketing | finance | agent_assist
retention: 30d | 24m | legal_hold | indefinite_policy
```

Treat `brand` as a governed organization child/scope where deployments operate
multiple brands. Tags guide policy, retrieval, retention, and redaction, but never
grant access.

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

## Starter and XSAMA profile details

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

## Product boundary summary

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

## Resolved implementation decisions

The following decisions resolve the final profile, identity, consistency, deletion,
parser, and telemetry details. They are normative.

### 1. Ship deployment profiles, not one mandatory infrastructure stack

The product contract remains provider-neutral. The reference deployment must not force every organization into the same broker or index.

| Profile | Eventing | Retrieval projection | Appropriate use |
|---|---|---|---|
| **Starter / homelab** | NATS JetStream | PostgreSQL + pgvector | One organization, low operational overhead, modest corpus, limited connectors |
| **Enterprise replay / CDC** | Apache Kafka + Debezium outbox | OpenSearch hybrid search | Long retention, high-volume CDC, broad connector ecosystem, many independent consumers |
| **Vector-specialist** | Either bus adapter | Qdrant plus lexical search provider | Dense retrieval/filtering is the bottleneck and a separate vector tier is justified |

For XSAMA's first controlled implementation, retain the starter profile: NATS JetStream and pgvector, with a strict event-bus/index adapter boundary. Do **not** make NATS or pgvector a product assumption. JetStream supplies durable streams, replay and at-least-once consumption; Kafka is the stronger default for deployments whose primary requirements are long replay, CDC, and a large connector ecosystem.[4][40]

The authoritative ledger stays PostgreSQL and raw evidence stays S3-compatible object storage in every profile. Search/vector systems are rebuildable projections only. OpenSearch becomes the reference scaled retrieval adapter because it combines lexical, semantic, and hybrid ranking; Qdrant remains a payload-filtered vector adapter, not an authorization authority.[25][41]

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

## Canonical records, identifiers, and state

Keep four identities separate:

1. `resource_id` identifies the stable logical resource (case, conversation,
   document, account, or person).
2. `revision_id` identifies one immutable observed version of that resource.
3. `artifact_id` plus `derivation_id` identifies an extraction, redaction,
   element set, chunk set, embedding generation, summary, or graph projection.
4. `event_id` identifies one immutable lifecycle fact or processing attempt.

Use UUIDv7/ULID event IDs. External URLs, mutable timestamps, source IDs, and
chunk IDs are not global event identities. Deterministic IDs are appropriate only
for replay-safe projections, for example:

```text
chunk_id = hash(
  revision_id,
  extraction_profile,
  element_locator,
  chunk_profile,
  ordinal
)
```

The canonical model includes organization, context space, principal, group,
resource, revision, evidence artifact, derived artifact, chunk, embedding
generation, context event, access relationship, delegation grant, policy decision,
audit event, ingestion job, and derivative manifest. Preserve both `valid_time`
(when the source says a fact applied) and `system_time` (when the platform observed
or processed it).

Every derived record carries at least:

```text
organization_id, context_space_id, resource_id, revision_id,
source_event_id, evidence object version + SHA-256,
extractor/chunker/embedder/model/profile versions,
policy/redaction version, authorization object reference,
trust, source authority, classification, allowed purpose,
confidence, review state, superseded/tombstoned/deleted state
```

The canonical lifecycle is:

```text
RECEIVED → VALIDATED → QUARANTINED | ACCEPTED
ACCEPTED → PROJECTING → INDEXED | FAILED
INDEXED → RECLASSIFIED | TOMBSTONED
TOMBSTONED → PURGE_PENDING → PURGED
```

Transitions are append-only. A tombstone or access revocation dominates an older
upsert after retries, replay, reconciliation, or restore.

## Deployment profiles and component authority

The product contract is provider-neutral; deployments choose a coherent profile:

| Profile | Durable event adapter | Retrieval projection | Intended use |
|---|---|---|---|
| Starter / XSAMA first slice | NATS JetStream | PostgreSQL + pgvector | Lowest operational overhead, modest corpus/connectors |
| Enterprise replay / CDC | Kafka KRaft + Debezium + Apicurio | OpenSearch hybrid search | Long replay, high-volume CDC, many consumers/connectors |
| Vector specialist | Either durable adapter | Qdrant plus lexical provider | Dense retrieval is a measured bottleneck |
| Analytics-heavy extension | Existing profile bus | ClickHouse analytics and optional retrieval experiment | High-volume audit/timeline analysis; never canonical |

PostgreSQL is the canonical catalog, lifecycle ledger, provenance index, policy
references, and processed-event ledger in every profile. S3-compatible storage is
the evidence plane. Search stores, vector stores, graph stores, caches, and
ClickHouse are rebuildable projections. Temporal is an optional/adopted workflow
controller for long-running enterprise ingestion and deletion; it is neither the
event ledger nor compliance archive.

Do not run two primary event buses or two authoritative search projections without
a written responsibility boundary, migration plan, and reconciliation tests.

## Identity, authorization, and PostgreSQL containment

The generic OIDC adapter validates discovery/JWKS, algorithm, issuer, audience,
expiry, not-before, authorized party/client, scopes, and organization claims. Human
identities are keyed by immutable `issuer + subject`, never email. Browser sessions
use authorization code with PKCE. SCIM may synchronize users and groups but is not
the live authorization decision point.

For the XSAMA profile, configure Logto as the OIDC provider and register the
gateway as an organization-level API resource. Derive organization only from the
validated token or a server-side mapping. A path organization is a consistency
assertion that must match the token; headers, prompts, filters, tags, and MCP tool
arguments never choose a tenant.[43]

Use small action scopes such as `context:search`, `context:read`,
`context:ingest`, `context:manage_policy`, `context:audit_read`, and
`context:request_access`; do not define wildcard data-plane scopes. Reference
organization roles may include `member`, `manager`, `knowledge_admin`, and
`compliance_reviewer`, with separately bounded service/agent roles. These are
coarse API ceilings, not resource visibility decisions.

Use one relationship authorization adapter in the first production slice:
OpenFGA. Model opaque objects such as `organization:<uuid>`, `team:<uuid>`,
`resource:<uuid>`, and principals. Resources have immutable parent-organization
relations; child chunks inherit visibility from their resource. Typical relations
include organization `member`/`manager`/`knowledge_admin`, team `member`, and
resource `reader`/`restricted_reader`/`can_read`/`can_manage`. Manager status alone
does not grant restricted access. Cross-organization sharing is an explicit,
expiring, approved exception.[44]

Use permission-aware discovery when tractable, mandatory tenant/scope predicates
during candidate search, then an exact batch authorization check before content
hydration. Never use stale authorization results for revocation-sensitive reads.
The authorization adapter's consistency contract is `min_latency`,
`at_least_as_fresh`, or `fully_consistent`. Persist sensitive grants and group
relations; do not depend on token-lifetime contextual tuples for immediate
revocation.

OPA is optional for contextual policy that returns disclosure obligations such as
redaction profile, allowed fields, approval requirement, result limit, purpose,
classification ceiling, or agent action ceiling. It does not duplicate the
relationship graph. Cedar may replace OPA for embedded schema-validated policy;
do not run OpenFGA, OPA, and Cedar as competing sources for the same decision.

Authenticate every agent worker/container as the separate `runtime` identity in
the delegation model. Add SPIFFE/SPIRE when service count or east-west trust
boundaries justify short-lived workload certificates; workload identity augments,
but never replaces, the end-user/agent authorization decision.[53]

PostgreSQL RLS is a tenant-containment backstop, not the per-resource sharing
authority. Every protected table includes non-null `organization_id`; composite
foreign keys prevent cross-organization parent references. Runtime roles are
non-owner and `NOBYPASSRLS`; protected tables use `FORCE ROW LEVEL SECURITY`.
Set tenant context transaction-locally after token validation so pooled connections
cannot leak a previous request's tenant.[42]

```sql
ALTER TABLE context.resource ENABLE ROW LEVEL SECURITY;
ALTER TABLE context.resource FORCE ROW LEVEL SECURITY;
REVOKE ALL ON context.resource FROM PUBLIC;

CREATE POLICY resource_org_isolation ON context.resource
  TO context_gateway
  USING (
    organization_id = current_setting('app.organization_id', true)
  )
  WITH CHECK (
    organization_id = current_setting('app.organization_id', true)
  );

-- At the start of every transaction; the value comes from validated identity.
SELECT set_config('app.organization_id', $1, true);
```

## Ingestion, processing, and schema contract

CloudEvents 1.x is the outer envelope. Versioned JSON Schema is the initial
enforceable payload contract; use Apicurio compatibility/validity rules when
multiple teams or enterprise profiles need runtime governance. AsyncAPI documents
channels but does not enforce domain compatibility.

Required envelope/domain fields are:

```text
event_id, event type/version, source, producer, organization_id,
context_space_id, resource_id, revision_id, occurred_at, observed_at,
classification, trust, source authority, schema ID/version,
correlation/causation IDs, trace context,
immutable evidence pointer + object version + SHA-256
```

Never put document bodies, raw PII, credentials, bearer tokens, or secret source
URLs in event payloads. Use bounded topic/stream families (`context.lifecycle.v1`,
`context.work.v1`, `context.derived.v1`, `context.audit.v1`, `context.dlq.v1`),
not one per tenant. Order lifecycle events by `(organization_id, resource_id)`;
make no global-order promise.

Intake writes source cursor/resource/revision state and an outbox row in one
PostgreSQL transaction after evidence quarantine and validation. The relay may
redeliver, so each materializer owns a durable unique
`(consumer_name, event_id)` receipt or deterministic projection key. External
effects use stable idempotency keys and recorded outcomes. Reconciliation compares
desired canonical state to projections and emits auditable repair events.

Schema rules:

- default to backward-transitive compatibility for event values;
- make additive optional changes only within a version;
- never reuse a field name/number with a different meaning;
- use a new event type/version and dual-read/write migration for breaking changes;
- version parser, OCR, chunk, embedding, redaction, and ACL profiles separately;
- quarantine unknown or malformed payloads instead of guessing.

Isolated workers execute:

```text
validate tenant/source/signature/schema
→ quarantine, ClamAV malware scan, MIME/archive/resource-limit checks
→ immutable evidence write and hash verification
→ extract/OCR with quality evidence
→ classify and create retrieval-safe redaction with Presidio/policy detectors
→ deterministic structure-aware chunking
→ embedding/index projection
→ ACL/provenance/final-state verification
→ make revision retrievable
```

Use Docling as the primary rich-document parser and Apache Tika as a broad-format
fallback behind `ParserProvider`; Unstructured remains a viable profile adapter.
Pin versions and enforce archive depth, decompressed bytes, file/page count, CPU,
memory, wall-clock, and no-network limits. Low-confidence extraction stays
quarantined or enters review.[48][49][62][63][64]

Chunks preserve page, section/title path, source element IDs, character offsets,
and table/image locators. Keep structured table data plus a readable rendition.
Index only policy-approved renditions. A new model or chunk policy creates a
parallel embedding generation and switches only after evaluation/backfill.

Temporal workflows, where enabled, run per revision or bounded job. Activities are
idempotent and heartbeat while long-running; coordinators use Continue-As-New
before history limits are approached.[46][47]

## Gateway API, MCP, retrieval, and context packets

REST is canonical. MCP is a thin adapter over the same application service and
policy path, not a second store. Initial REST operations are constrained business
operations:

```text
POST /v1/organizations/{orgId}/context:search
GET  /v1/organizations/{orgId}/context/resources/{resourceId}
POST /v1/organizations/{orgId}/context/access-requests
POST /v1/organizations/{orgId}/context/sources
GET  /v1/organizations/{orgId}/context/audit
```

Do not expose arbitrary SQL, caller-defined vector predicates, unrestricted URL
fetching, raw authorization tuple administration, filesystem access, or a generic
HTTP proxy. Start MCP read-only with `context.search`, `context.get`,
`context.brief`, and `context.request_access`. The protected MCP endpoint publishes
OAuth protected-resource metadata at `/.well-known/oauth-protected-resource`,
returns a `401` bearer challenge with only the required scopes, validates its own
resource audience, and never passes client tokens to downstream services. Prefer
pre-registered enterprise clients; do not enable dynamic client registration by
default merely to accommodate unknown clients.[26][35]

Every search executes:

```text
authenticate principal and runtime
→ resolve organization, delegation, purpose, and consistency requirement
→ scope/action gate
→ relationship authorization and contextual disclosure policy
→ RLS transaction context
→ mandatory server-generated predicate
→ lexical/vector candidate retrieval
→ exact current authorization/state check
→ hydrate only allowed retrieval-safe content
→ redaction/output DLP
→ bounded cited context packet
→ append-only audit event
```

Indexes return stable candidate IDs plus minimal partition metadata, not plaintext.
Never fetch global top-K and filter afterward. Cache keys include organization,
principal/delegation, authorization revision, policy revision, purpose, and query
intent; invalidation must meet the measured revocation SLO. Valkey may hold
non-sensitive or authorization-keyed caches, but it is never canonical and a cache
hit never bypasses current revocation/state checks.

The context packet includes summary, facts, relevant timeline entries,
stakeholders, citations (`resource/revision/page/section`), applied redactions,
policy/authz revision, audit ID, confidence/review state, and agent action
restrictions. It does not expose denied-resource metadata, raw database rows, or
opaque vector IDs.

## Evidence, provenance, deletion, and audit

Each revision has a derivative manifest that enumerates raw object versions,
renders, extracted elements, chunks, embedding generations, summaries, graph facts,
caches, exports, and search rows. Citation-capable chunks preserve:

```text
chunk_id, organization_id, context_space_id, resource_id, revision_id,
source connector + external locator, evidence URI/version/SHA-256,
observed/source-modified timestamps, extractor/config/artifact hash,
page/section/element/character locator, redaction policy,
chunk and embedding profiles, authorization reference,
event/causation/workflow IDs, lifecycle state
```

Deletion is a saga:

1. authorize and record scope, requester, policy, legal-hold result, and deadline;
2. immediately revoke gateway visibility;
3. enumerate every derivative and copy from the manifest;
4. remove projections/caches first and prove zero retrievable results;
5. erase evidence according to retention/legal-hold policy;
6. emit a signed completion or blocked manifest and reconcile periodically.

Object-store versioning, legal holds, and WORM retention mean a missing current key
does not prove physical erasure. Treat retention and erasure as explicit legal
policies; use per-tenant envelope-key destruction where appropriate and verified.
Projection deletion lag never delays access revocation.[38][50][51]

Audit events include request/trace/organization IDs; subject, actor, owner,
runtime, OAuth/MCP client, and delegation grant; action/scopes; opaque resource
counts/IDs; authorization model/revision/consistency and result; policy decision
and redaction profile; token fingerprint/JTI hash; latency/error/output size; and
deletion or administrative outcome. Do not log raw queries, prompts, outputs,
tokens, source content, or secret-bearing tool arguments by default. Mask OPA and
telemetry exports before enabling them.[45][52]

## Specialist component adjudication

| Component | Authoritative disposition |
|---|---|
| PostgreSQL + pgvector | Canonical foundation and starter retrieval; RLS tenant backstop |
| OpenSearch | Reference enterprise hybrid retrieval projection |
| Qdrant | Optional vector-specialist projection after measurement; never direct or authoritative. Self-hosted instances require explicit TLS/API-key/network hardening.[68] |
| Weaviate | Alternative if native tenant shards/RBAC justify operating another vector platform.[61] |
| ClickHouse | Audit/timeline analytics and optional measured retrieval experiment; never truth |
| Haystack | Preferred controlled retrieval/pipeline library, not a shared data plane |
| LlamaIndex | Alternative connector/ingestion library behind the same contracts |
| Graphiti | Optional derived temporal graph for bounded domains; not ACL or truth; its MCP integration is not a production security boundary |
| Zep Cloud | Excluded from local-data deployments; its managed governance features do not transfer to self-managed Graphiti.[70] |
| Mem0 | Optional isolated user/agent preference memory; not shared organizational knowledge |
| Letta | Isolated stateful-agent runtime only; controller authorization and tenant isolation remain mandatory |
| R2R | Proof-of-concept only until repository maintenance and exact collection/access enforcement are revalidated |
| Dify | Optional consumer/UI through gateway; verify multi-tenant license and propagate end-user identity rather than trusting a static external-knowledge key.[58][69] |
| OpenMetadata/DataHub | Governance catalog projections/integrations, not runtime context authority |
| Airbyte/NiFi/Redpanda Connect | Optional edge connectors, never the canonical/policy layer |

Haystack/LlamaIndex, Graphiti, Mem0, Letta, R2R, and Dify must not become shortcuts
around the gateway, canonical records, delegation model, or authorization checks.
[54][55][56][57][58][59][60]

## Operational non-negotiables

1. Broker producer idempotency or transactions do not provide end-to-end exactly
   once across PostgreSQL, S3, parsers/models, indexes, and external APIs.
2. Enterprise Debezium deployments budget and alert on PostgreSQL replication-slot
   WAL retention; an outage can exhaust disk, while an aggressive retention cap can
   invalidate the slot and force resnapshot.[66]
3. Do not clean outbox rows based only on an application publish acknowledgement.
   Retain them through a durable CDC high-water mark and reconciliation window.
4. Kafka-compacted current-state topics never replace non-compacted lifecycle and
   audit history.
5. Broker/schema migrations require dual read/write, backfill, compatibility tests,
   and a deprecation window; registry acceptance alone cannot preserve semantics.
6. S3 versioning, delete markers, WORM, and legal holds distinguish “not visible”
   from “physically erased.”
7. Search filters and cache partitioning must prevent tenant, score, count,
   pagination, citation, and answer-cache side channels.
8. ACL changes synchronously affect gateway decisions; asynchronous index updates
   cannot be the revocation authority.
9. Projection mutation/delete lag never delays gateway denial.
10. Hostile documents require malware scanning, parser sandboxing, archive/PDF
    expansion limits, no macros, no network, and reproducible pinned dependencies.
11. Empty extraction, OCR noise, stripped tables, duplicate connector pages, stale
    cursors, and clock skew are observable quality states, not silently indexed data.
12. Re-embedding/profile changes need capacity plans, parallel generations, canary
    evaluation, rollback, and provenance-aware retirement of old indexes.
13. Long Temporal workflows heartbeat and Continue-As-New; workflow history is not
    a permanent compliance ledger.
14. Telemetry is a disclosure surface: scrub prompts, outputs, PII, credentials,
    baggage, and plugin fields before export.
15. Restore drills recover PostgreSQL, evidence mappings/object versions, event
    history, schema contracts, authorization models/tuples, policies, and secrets as
    one measured RPO/RTO scenario; replay tombstones before serving restored data.
16. Redpanda is a profile alternative only after accepting its source-available BSL
    terms and validating Kafka/Debezium/backup compatibility.[65]

Use OpenTelemetry and the existing Langfuse installation for redacted operational
and retrieval evaluation metadata once its OIDC integration is healthy. Content
capture remains opt-in. Maintain versioned retrieval, authorization, prompt-
injection, deletion, and consent/suppression fixtures; marketing policy must observe
consent changes before any send.[52][67]

## Delivery sequence

1. Version the canonical record/event schemas, REST/OpenAPI, AsyncAPI, taxonomy,
   authorization fixtures, and deletion state machine.
2. Build synthetic employee, manager, customer, agent, restricted-note, revocation,
   and tombstone fixtures.
3. Implement the gateway with generic OIDC, OpenFGA, PostgreSQL/RLS, evidence
   storage, structured audit, and no agent bypass.
4. Complete one source end-to-end (Chatwoot for XSAMA), including quarantine,
   parser isolation, outbox, replay, retrieval, citation, and deletion.
5. Prove cross-tenant denial, delegation narrowing, revocation consistency,
   replay determinism, prompt-injection containment, and restore safety.
6. Enable read-only REST retrieval, then the thin remote MCP adapter.
7. Add write/send actions only behind explicit scopes, budgets, and approval.
8. Add Kafka/OpenSearch/Qdrant/Temporal/graph or catalog components only when
   measured scale, replay, workflow, or query requirements justify the profile.

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
[41] https://docs.opensearch.org/latest/vector-search/ai-search/hybrid-search/index/
[42] https://www.postgresql.org/docs/current/ddl-rowsecurity.html
[43] https://docs.logto.io/authorization/validate-access-tokens
[44] https://openfga.dev/docs/interacting/relationship-queries
[45] https://www.openpolicyagent.org/docs/management-decision-logs
[46] https://docs.temporal.io/activity-definition
[47] https://docs.temporal.io/workflow-execution/limits
[48] https://tika.apache.org
[49] https://docs.unstructured.io/open-source/core-functionality/chunking
[50] https://docs.min.io/aistor/administration/object-locking-and-immutability
[51] https://clickhouse.com/docs/concepts/features/operations/delete/overview
[52] https://opentelemetry.io/docs/security/handling-sensitive-data
[53] https://spiffe.io/docs/latest/spire-about/use-cases
[54] https://docs.haystack.deepset.ai/docs/document-store
[55] https://docs.llamaindex.ai/en/stable/module_guides/loading/ingestion_pipeline
[56] https://docs.mem0.ai/open-source/features/rest-api
[57] https://docs.letta.com/platform/app-server/integration-patterns
[58] https://github.com/langgenius/dify/blob/main/LICENSE
[59] https://github.com/getzep/graphiti
[60] https://github.com/SciPhi-AI/R2R
[61] https://docs.weaviate.io/weaviate/manage-collections/multi-tenancy
[62] https://docling-project.github.io/docling/
[63] https://docs.clamav.net
[64] https://github.com/microsoft/presidio
[65] https://docs.redpanda.com/streaming/current/get-started/licensing/overview
[66] https://www.postgresql.org/docs/current/logicaldecoding-explanation.html
[67] https://langfuse.com/docs/observability/get-started
[68] https://qdrant.tech/documentation/security/
[69] https://docs.dify.ai/en/cloud/use-dify/knowledge/external-knowledge-api
[70] https://help.getzep.com/zep-vs-graphiti
