# Security and Governance Recommendation: Organization Context Layer

**Scope:** research and architecture recommendation only. No systems were deployed or changed.[unverified]

## Decision

Build one **Context Gateway** as the sole public API and the sole remote MCP server. Put Logto at the identity edge, PostgreSQL RLS at the tenant data boundary, and OpenFGA at the fine-grained authorization decision point. Use OPA only for policy-as-code decisions that are attribute-, risk-, and governance-oriented (classification, redaction profile, agent/tool capability, approval), rather than as the document-sharing graph. This separates coarse RBAC, relationship-based access, and database containment without introducing a proprietary authorization service.[unverified]

Logto organization API resources are designed for tenant-scoped APIs: a token can carry API audience plus organization context, and organization roles can be assigned to users and M2M clients. The gateway must validate signature/JWKS, issuer, expiry, audience, scopes, and the `organization_id`/organization audience before it derives any tenant context.[11][12][85]

```text
Human client or agent client
          │  Logto OIDC/OAuth access token
          ▼
   Context Gateway ── same policy path ── Remote MCP adapter
          │  (PEP: token validation, tenant binding, audit)
          ├── OpenFGA (PDP: subject ↔ org/team/document relations)
          ├── OPA (PDP: classification, disclosure, tool/agent risk)
          ├── PostgreSQL + pgvector (RLS tenant backstop; encrypted content)
          └── audit/telemetry sink (append-only events, redacted)

Ingestion workers ─────► classification/redaction ► PostgreSQL + FGA tuples
```

**Authority boundaries:** Logto authenticates people and services and grants coarse API capabilities; OpenFGA decides which organization objects a subject may read; OPA decides whether the permitted representation may be disclosed and under which redaction/approval policy; PostgreSQL prevents a gateway query defect from crossing tenants. The model is *deny by default* at every boundary.[unverified]

## Recommended identity and authorization model

### 1. Logto: authentication, organization tenancy, and coarse permissions

Register the gateway API as an **organization-level Logto API resource**. Define small, action-oriented scopes such as `context:search`, `context:read`, `context:ingest`, `context:manage_policy`, `context:audit_read`, and `context:request_access`; avoid `admin`, `all`, or wildcard scopes at the data plane. Define organization-template roles such as `member`, `manager`, `knowledge-admin`, `compliance-reviewer`, and separately scoped M2M/agent roles. Logto supports assigning organization roles to M2M applications as well as users.[11][13][85]

For each request, accept the organization only from the validated access token or a server-side mapping of it. If the REST path contains `/organizations/{orgId}`, require it to exactly equal the token’s `organization_id` and organization audience; never use a caller-supplied header, prompt field, or tool argument to select a tenant. Logto explicitly calls for validation of audience, expiration, organization context, and scopes for organization-protected APIs.[12]

Use OAuth resource indicators when minting tokens (`resource=https://context.example.com/api` or the registered gateway indicator). Resource indicators let the authorization server bind the token audience to the intended protected resource, rather than treating scopes as resource identity.[16]

### 2. OpenFGA: fine-grained relationship authorization

Use a single OpenFGA logical store at first, backed by a **dedicated database/schema and database role within the existing PostgreSQL cluster**. This is cost-efficient while isolating FGA operational data and credentials from application content. OpenFGA supports PostgreSQL, OIDC authentication, TLS, read replicas, and a higher-consistency option for sensitive read-after-write/revocation paths.[19]

Model immutable, opaque identifiers only, for example `organization:<uuid>`, `team:<uuid>`, `document:<uuid>`, `chunk:<uuid>`, `user:<Logto-sub>`, and `service:<client-id>`. Every document has an immutable parent organization relation; chunks inherit access from their document. Typical relations are:

- Organization: `member`, `manager`, `knowledge_reader`, `knowledge_admin`.
- Team: `member`.
- Document: `parent_org`, `reader` (direct/team/user set), `manager_reader`, `restricted_reader`, `can_read`, and `can_manage`.
- Chunk: `parent_document`, `can_read` derived from its parent document.

This makes the intended hierarchy explicit: a manager can inherit permitted organization content but does **not** automatically read `restricted_reader` material unless the policy says so. Cross-organization sharing is an explicit exception relation, normally disabled; if it is enabled, require data owner and compliance approval, a finite expiry, and a dedicated audit event. Cedar’s security guidance similarly recommends unique, immutable, non-recyclable identifiers for authorization entities.[25]

The gateway should use `ListObjects`/permission-aware search to establish an authorized object set when that set is tractable, then issue a tenant-filtered retrieval query. It must **BatchCheck every candidate chunk/document that could reach the model or response** before disclosure; FGA’s Check, ListObjects, and BatchCheck APIs are purpose-built for this, and BatchCheck supports correlated multi-decision responses.[20] Do not return vector hits simply because their similarity score is high.

For newly granted/revoked permissions and just-in-time emergency restrictions, bypass authorization-result caches and use higher-consistency FGA reads. Cache only immutable metadata and short-lived negative/allow results where the threat model tolerates revocation latency; include policy/model version and relationship-change version in cache keys.[19]

Use Logto claims as FGA contextual tuples only for stable, low-risk attributes such as a non-sensitive department claim. Contextual tuples are not persisted, work only for particular FGA query APIs, and retain the token’s effective privileges until it expires; they are a poor basis for revocation-sensitive authorization-aware indexes.[86] Persist sensitive groups, access grants, expiry, and exceptions as authoritative OpenFGA tuples instead.

### 3. OPA: governance and selective-disclosure policy, not the sharing graph

OPA is the recommended companion policy engine when policy must return more than allow/deny: `allow`, `redaction_profile`, `allowed_fields`, `approval_required`, `max_results`, `agent_capability`, `reason_code`, and `policy_revision`. OPA can run locally as a sidecar/daemon, expose named decisions via REST, and distribute policy/data as bundles.[23]

Examples appropriate for OPA:

- `restricted` classification requires `restricted_reader` *and* an approved purpose; managers receive a redacted derivative by default.
- A `people-manager` may see team aggregate context but not salary, health, legal, credential, or investigation fields.
- An ingestion source with an unreviewed prompt-injection flag can be searchable only as quarantined metadata, not supplied as model context.
- An agent has a configured capability ceiling (`search_public_internal`, no restricted content, no writes) even when the initiating user has broader rights.
- `context:manage_policy`, cross-tenant share, retention deletion, export, or write actions require a separate approval workflow.

Keep policy source, tests, bundle revision, approver, and rollback plan in version control. OPA decision logs contain the queried policy, decision ID, bundle revision, input, result, trace/span IDs, and can erase or mask fields by JSON Pointer before export.[24] Configure masking before enabling remote decision logs; raw prompts, bearer tokens, document text, PII, and secret-like tool arguments must not be stored in authorization telemetry.

### 4. Cedar alternative and why it is not the primary recommendation

Cedar is a credible alternative if the organization prefers an embedded, schema-validated ABAC/RBAC policy library and can consistently supply all required principal/resource entities. Its default-deny and forbid-overrides-permit semantics, plus schema validation, are strong governance characteristics.[25]

Do **not** choose Cedar as the primary data-plane PDP for this design unless the sharing model is mostly attributes and roles. It does not remove the need to maintain relationship/entity data, and OpenFGA is a better fit for dynamic organizational membership, manager/team inheritance, per-document sharing, bulk checks, and permission-aware discovery. A defensible alternate is **Cedar instead of OPA** for application-local disclosure rules, but avoid running OpenFGA + OPA + Cedar for the same decision: that creates duplicated policy truth and audit ambiguity.

## Authorization-aware RAG and selective disclosure

### Secure retrieval sequence

1. Validate the Logto JWT and derive `actor`, `client_id`, `org`, scopes, authentication strength, and request ID. Enforce scope before any retrieval.
2. Normalize the query; impose per-subject, per-agent, and per-organization budgets. Do not embed or log raw secrets if the query contains them.
3. Resolve FGA permission candidates (`can_read`) for the subject and organization, or retrieve a bounded tenant-only candidate set and BatchCheck each result.
4. Query PostgreSQL/vector search with a mandatory tenant predicate and RLS session context. The vector index must be treated as an index, not as authorization.
5. Apply OPA disclosure policy to each authorized candidate. Select a pre-generated derivative (`full`, `manager-redacted`, `member-redacted`, `metadata-only`) rather than relying solely on runtime regular expressions.
6. Only then supply permitted, redacted chunks to the model, with source ID, classification, and an `untrusted_content` marker. Apply a final output DLP/redaction check and return safe citations rather than denied metadata.
7. Emit an audit event containing identifiers, policy/model revisions, decisions, counts, and hashes—not raw document text or bearer tokens.

Store lineage for every source and derivative: source system, source locator, content hash, owner, `logto_org_id`, document/chunk IDs, classification, sensitivity tags, legal/retention state, FGA object ID, OPA policy version, redaction profile, embedding model/version, and ingestion scan status. Re-embed only approved derivatives; do not embed material that an LLM must never receive.

Treat retrieved documents, MCP tool results, emails, and web content as **untrusted data**, not instructions. OWASP notes that RAG and fine-tuning do not fully mitigate prompt injection and recommends least privilege, deterministic validation, segregation of external content, human approval for high-risk actions, and adversarial testing.[28] OWASP also identifies sensitive-information disclosure and excessive agency as risks when applications give LLMs broad data and tool permissions.[29][30]

## PostgreSQL tenant backstop

Put `logto_org_id NOT NULL` on every content, chunk, embedding, ingestion-job, access-request, and audit-reference row; enforce composite foreign keys so a chunk cannot point at a document in another organization. Make the gateway’s content database role a non-owner with `NOBYPASSRLS`, and do not grant it direct access to unfiltered views. PostgreSQL RLS is default-deny when enabled with no applicable policy, but superusers, `BYPASSRLS` roles, and typically table owners bypass it; use a separate migration owner and `FORCE ROW LEVEL SECURITY` on protected tables.[22]

Illustrative pattern (not executed):

```sql
-- Migration/admin role owns the table. The runtime gateway role does not.
ALTER TABLE context.document ENABLE ROW LEVEL SECURITY;
ALTER TABLE context.document FORCE ROW LEVEL SECURITY;
REVOKE ALL ON context.document FROM PUBLIC;
GRANT SELECT, INSERT, UPDATE, DELETE ON context.document TO context_gateway;

CREATE POLICY document_org_isolation ON context.document
  TO context_gateway
  USING (
    logto_org_id = current_setting('app.logto_org_id', true)
  )
  WITH CHECK (
    logto_org_id = current_setting('app.logto_org_id', true)
  );

-- At the start of *each* gateway transaction, after JWT validation:
SELECT set_config('app.logto_org_id', $1, true); -- $1 derived from token, SET LOCAL
```

Use transaction-scoped `set_config(..., true)`/`SET LOCAL`, including with connection pools, so a previous request’s tenant cannot bleed into a reused connection. The application, not the client, sets the value only after validation. Avoid RLS policies that repeatedly join mutable authorization tables on every row without a carefully tested concurrency design; PostgreSQL documents potential race/leak considerations for cross-table policy expressions.[22]

RLS guarantees tenant containment, **not** every per-user document entitlement. OpenFGA remains authoritative for document/chunk access. Conversely, OpenFGA does not replace a storage-level backstop. This deliberate overlap protects against different failure modes.

## API and MCP shape

### REST API

Expose only constrained business operations, for example:

- `POST /v1/organizations/{orgId}/context:search` — `context:search`; returns redacted, authorized chunks and citations.
- `GET /v1/organizations/{orgId}/context/resources/{resourceId}` — `context:read`; FGA check plus disclosure decision.
- `POST /v1/organizations/{orgId}/context/access-requests` — request, not grant, elevated access.
- `POST /v1/organizations/{orgId}/context/sources` — ingestion only, separate scope/service identity.
- `GET /v1/organizations/{orgId}/context/audit` — narrow audit-read scope and audit-specific FGA/OPA policy.

Do not expose arbitrary SQL, unrestricted vector filters, “fetch URL,” raw FGA tuple administration, or a generic proxy tool to agents. The `orgId` is a checked consistency assertion, never authority.

### Remote MCP adapter

Implement the MCP server as a thin HTTP adapter over the exact same gateway authorization/retrieval service—never as a second context store or a bypass path. In the current MCP authorization specification, a protected MCP server is an OAuth resource server and must implement OAuth Protected Resource Metadata; clients use it to discover the authorization server, while servers should challenge only the scopes necessary for the operation.[83][18]

Publish `/.well-known/oauth-protected-resource` for the MCP protected resource and return a `401` `WWW-Authenticate: Bearer` challenge that names its resource metadata and minimal scopes. Use Logto as the authorization server if its discovered OIDC/OAuth metadata and client registration behavior have been verified against target MCP clients. Prefer pre-registering trusted enterprise MCP clients in Logto. MCP permits pre-registration, whereas dynamic registration is optional/deprecated in favor of client-ID metadata documents; do not enable dynamic registration by default just to support unknown clients.[83]

Expose a small read-only initial MCP tool catalog:

- `context.search` → REST search path
- `context.get` → REST resource path
- `context.request_access` → creates a reviewable request, never changes privileges

Do not expose write, delete, policy-admin, arbitrary HTTP, arbitrary filesystem, or raw SQL tools to general agents. Disable MCP resources, prompts, and sampling unless a justified use case needs them. Every tool invocation must go through the same token validation, FGA check, OPA disclosure decision, RLS transaction, content scan, and audit event as REST.

The MCP server must validate that received access tokens were issued **for the MCP/gateway resource** and must not pass an MCP client token to downstream APIs. MCP calls this token passthrough an explicit anti-pattern because it weakens audience boundaries, accountability, and auditability.[84] Instead, the gateway calls FGA, PostgreSQL, OPA, and source connectors using its own narrowly scoped service identity while carrying a signed internal actor envelope for audit and policy evaluation.

## Agents and service identities

Give every workload its own Logto M2M application, API audience, scopes, organization memberships, secret/credential rotation schedule, and audit identity. An ingestion worker needs only its organization-scoped source/ingestion permissions; a retrieval service needs only read decisions and database access; an audit exporter must not retrieve content. Logto’s M2M flow and roles are intended for protected service-to-service API access.[13]

For internal transport, add SPIFFE/SPIRE when service count or trust boundaries warrant it. SPIRE attests workloads and delivers workload-specific, short-lived, automatically rotated X.509-SVIDs for mTLS; this avoids static east-west credentials and authenticates the caller workload, but does not replace the end-user actor or authorization decision.[26]

For an agent acting for a user, calculate effective privilege as:

```text
effective access = user FGA/disclosure rights
                 ∩ agent registered capability ceiling
                 ∩ task/session grant
                 ∩ requested API scope
```

Record both `actor_sub` (user), `client_id`/agent ID, and gateway workload identity. The agent’s M2M role is a ceiling, not a universal superuser. For background automation with no user, use only the agent/service’s own narrowly granted FGA relations. If later adopting OAuth token exchange for formal delegated downstream credentials, use RFC 8693 semantics and preserve the actor (`act`) claim, but first verify that the chosen Logto deployment supports the exact exchange profile; no Logto token-exchange support was assumed here.[87]

## Governance, audit, and evaluation

Create an append-only authorization/audit event stream, written through an outbox and retained separately from content. Each event should include:

- timestamp, request ID, trace ID, organization ID;
- actor subject/type, OAuth client ID, MCP client/tool ID, gateway/SPIFFE workload ID;
- action, scope set, source/resource/chunk opaque IDs and counts;
- FGA model ID/relationship change version and allow/deny result;
- OPA decision ID, bundle revision, disclosure/redaction profile, reason code;
- token fingerprint/JTI hash (not token), query hash/length/classification (not raw query by default), latency, error, and output byte/token count.

Use OpenTelemetry trace context to correlate the gateway, FGA/OPA calls, retrieval, model call, and audit event. Its log model supports trace and span IDs, while the GenAI convention registry identifies retrieval, model, tool, evaluation, and usage fields—but warns that prompts, outputs, tool arguments, and results can contain sensitive data, so content capture must be opt-in and redacted.[27]

Required gates before production rollout:

1. **Authorization-model tests:** FGA positive/negative test fixtures covering every relation, cross-org denial, manager restriction, expiring share, departed user, service identity, and `ListObjects` result. OpenFGA recommends testing every model before deployment and supports tests for Check/ListObjects/ListUsers.[21]
2. **Policy tests:** OPA unit/decision tests for each classification and redaction outcome; schema-validated data contracts; policy-review ownership and versioned approvals.
3. **Storage tests:** direct database queries as the runtime role with no tenant setting, wrong tenant setting, pooled-connection reuse, table-owner behavior, and migration-role behavior. All must fail closed except explicitly privileged break-glass workflows.
4. **RAG leakage tests:** seed canary secrets and identical terms across two organizations; prove that direct retrieval, vector retrieval, pagination, citations, summaries, cache hits, error messages, analytics, and model outputs never reveal another tenant’s data.
5. **Agent/MCP adversarial tests:** direct/indirect prompt injection inside retrieved content, tool-call parameter tampering, token audience confusion, redirect/client-registration attacks, SSRF attempts, and unauthorized write attempts.
6. **Operational SLOs:** authorization-deny rate by policy version, zero unauthorized disclosure in canary tests, audit-event completeness, policy/FGA availability, decision latency, post-revocation propagation, redaction failure rate, and cost per authorized answer.

## Delivery plan (no deployment performed)

1. Define the resource taxonomy, classification matrix, Logto organization roles/scopes, and explicit owner/approver/RACI table.
2. Build the gateway REST path with JWT validation, tenant transaction context, RLS, structured audit, and no agent-specific bypass.
3. Model FGA relations and tests; integrate batch authorization at retrieval and direct-read boundaries.
4. Add OPA disclosure policy and derivative-redaction ingestion pipeline.
5. Add the remote MCP adapter only after the REST policy path is proven; start read-only and pre-registered clients only.
6. Introduce SPIFFE/SPIRE and formal delegation/token-exchange only when the number of internal workloads or external downstream APIs justifies the operational complexity.

## Sources

[11] https://docs.logto.io/authorization/organization-level-api-resources
[12] https://docs.logto.io/authorization/validate-access-tokens
[13] https://docs.logto.io/quick-starts/m2m
[16] https://www.rfc-editor.org/rfc/rfc8707.html
[18] https://www.rfc-editor.org/rfc/rfc9728.html
[19] https://openfga.dev/docs/getting-started/setup-openfga/configure-openfga
[20] https://openfga.dev/docs/interacting/relationship-queries
[21] https://openfga.dev/docs/modeling/testing
[22] https://www.postgresql.org/docs/current/ddl-rowsecurity.html
[23] https://www.openpolicyagent.org/docs/integration
[24] https://www.openpolicyagent.org/docs/management-decision-logs
[25] https://docs.cedarpolicy.com/other/security.html
[26] https://spiffe.io/docs/latest/spire-about/use-cases
[27] https://opentelemetry.io/docs/specs/otel/logs/data-model
[28] https://genai.owasp.org/llmrisk/llm01-prompt-injection
[29] https://genai.owasp.org/llmrisk/llm02-insecure-output-handling
[30] https://genai.owasp.org/llmrisk/llm06-sensitive-information-disclosure
[83] https://modelcontextprotocol.io/specification/latest/basic/authorization
[84] https://modelcontextprotocol.io/docs/latest/tutorials/security/security_best_practices
[85] https://docs.logto.io/organizations/organization-management
[86] https://openfga.dev/docs/modeling/token-claims-contextual-tuples
[87] https://www.rfc-editor.org/rfc/rfc8693.html
