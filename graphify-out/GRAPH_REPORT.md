# Graph Report - gontext  (2026-09-02)

## Corpus Check
- 196 files · ~129,634 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 3575 nodes · 6191 edges · 286 communities (269 shown, 17 thin omitted)
- Extraction: 99% EXTRACTED · 1% INFERRED · 0% AMBIGUOUS · INFERRED: 82 edges (avg confidence: 0.79)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `7c5e7e68`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- Server
- time.Time
- mapping.go
- ADR Index
- Context Fabric Client SDK
- Service
- properties
- Store
- Memory
- context.Context
- NewEventID
- Worker
- CLI Commands
- testing.T
- wire
- .Intake
- enum
- Bounded Adaptive Retrieval Routing (ADR 0015)
- Service
- oidc.go
- suite.go
- properties
- config.go
- properties
- properties
- Store
- Context Packet Schema
- Helm Chart Template
- buckets
- properties
- properties
- TypeScript Package Config
- context-fabric/main.go
- replay_window_seconds
- Bus
- required
- properties
- properties
- chatwoot/connector.go
- required
- AgentCredential
- Config
- EvidenceStore
- Principal
- required
- required
- Graph Edge Schema
- Graph Edge Sync Schema
- Rate Quota Schema
- properties
- $defs
- properties
- null
- enum
- properties
- required
- properties
- Brief Request Schema
- properties
- enum
- properties
- webhookSigning
- OIDC Client Config Alt
- Schema Migration Config
- properties
- Storage Capability Schema
- OIDC Claim Mapping
- enum
- enum
- enum
- properties
- Content Blob Schema
- properties
- properties
- properties
- properties
- NewLimiter
- items
- Field Transform Schema
- plugin-manifest.json
- Plugin Version Schema
- serviceAccount
- properties
- required
- properties
- properties
- enum
- Plugin Capabilities Schema
- Graph Citation Schema
- Mapping Spec Definitions
- Tenant Policy Constraints
- Dry Run Edge Schema
- Direct Access Constraints
- taxonomy.json
- port
- roles
- Load
- TypeScript Compiler Config
- enum
- required
- Evidence Object Schema
- Archive Manifest Checksums
- properties
- capabilities
- properties
- properties
- properties
- setup
- required
- Citation Schema
- enum
- properties
- enum
- enum
- properties
- required
- enum
- required
- type
- Usage Budget Limits
- type
- type
- enum
- properties
- database
- metadata
- jetstream
- Change Event Schema
- Resource Change Events
- enum
- tombstone
- visibility_mapping_policy
- properties
- Health Probes Schema
- properties
- Auth Credential Tests
- required
- required
- NewOIDC
- Redaction
- error
- Authority Constraints Schema
- timestamps
- Forbidden Direct Access
- Emitted Event Types
- Tags Schema
- enum
- Classification Levels Enum
- download_uri
- logging.go
- Prediction Scoring Tool
- types.go
- OpenAPI Generation Script
- $defs
- type
- Delegation Grant Schema
- Event Envelope Schema
- required
- properties
- export-manifest.json
- ValidateWebhookTarget
- EvidenceRouter
- enum
- OpenAPI Contract
- enum
- Review State Enum
- Source Registration Schema
- Retention Policy Enum
- Trust Level Enum
- networkPolicy
- required
- Runbook: Restore
- Deploy Context Fabric — starter Compose (production)
- ports.go
- ADR 0013: Knowledge-Graph-First
- required
- Purpose Allowlist Schema
- Send Policy Enum
- enum
- Trust Tier Enum
- Purpose Allowlist Schema
- required
- properties
- values.schema.json
- connectionMode
- Runbook: upgrade and rollback
- enum
- record.json
- required
- migrate.go
- Purpose Allowlist Schema
- Tags List Schema
- Example ID Pattern
- type
- Runtime Enum
- Evidence Provenance Enum
- properties
- required
- enum
- Pagination Cursor Schema
- Event ID Schema Field
- Profile Version Schema Field
- Actor Schema Field
- revoked_at
- Owner Schema Field
- Runtime Schema Field
- Subject Schema Field
- ID Schema Field
- Idempotency Key Schema Field
- type
- Source Schema Field
- Confidence Score Schema Field
- Organization ID Schema Field
- Resource ID Schema Field
- Revision ID Schema Field
- enum
- visibility_ref
- synthetic/connector.go
- required
- Runbook: backup and restore
- Model ID Schema Field
- properties
- Created At Schema Field
- allowed_actions
- source_revision
- Content Locator Schema Field
- Context Space ID Schema Field
- Data Content Type Schema Field
- Data Schema Field
- Observed At Schema Field
- Occurred At Schema Field
- Producer Schema Field
- Resource ID Schema Field
- Revision ID Schema Field
- Schema ID Schema Field
- secrets
- Spec Version Schema Field
- Time Schema Field
- Type Pattern Schema Field
- LocalProvider
- system_time
- Parent Resource ID Schema Field
- Visibility Ref Schema Field
- 02_gateway_password.sh
- occurred_at
- Retention Policy ID Schema Field
- organization_id
- AsAPIError
- grant_id
- Title Schema Field
- subject
- traceparent
- content_locator
- source_event_id
- Resource ID Schema Ref
- Resource Type Schema Ref
- trust
- enum
- dependencies
- backup.sh
- Reconcile Script
- restore.sh
- Secrets Schema Definition
- Context Fabric Repository
- required
- validate-contracts.sh
- validate-profiles.sh
- endpoint
- Helm values.yaml (scaled profile)
- ADR 0016: Compose-first production release
- Runbook: AuthZ outbox backlog
- Runbook: Backup failure
- Runbook: OpenFGA outage
- Runbook: `/health/ready` returns false
- Context Fabric Threat Model (STRIDE)
- openfga
- Skill: decision-diagnosis
- dependencies
- upgrade.sh
- controlled_taxonomy
- role_secret_ref
- content_locator
- compose-smoke.sh
- compose-starter-preflight.sh
- purpose_allowlist
- source_authority

## God Nodes (most connected - your core abstractions)
1. `Store` - 74 edges
2. `ApplicationService` - 65 edges
3. `Server` - 56 edges
4. `Store` - 54 edges
5. `ErrNotFound()` - 51 edges
6. `ErrValidation()` - 48 edges
7. `Credentials` - 48 edges
8. `writeErr()` - 44 edges
9. `writeJSON()` - 43 edges
10. `wire()` - 38 edges

## Surprising Connections (you probably didn't know these)
- `ADR 0004: Generic Intake and MappingSpec` --references--> `mapping_specs`  [EXTRACTED]
  docs/adr/0004-generic-intake-and-mapping.md → contracts/jsonschema/export-manifest.json
- `AsyncAPI Contract` --references--> `ADR 0012: V1 Boundaries`  [EXTRACTED]
  contracts/asyncapi/asyncapi.yaml → docs/adr/0012-v1-boundaries.md
- `Context Fabric README` --references--> `Helm Chart.yaml`  [EXTRACTED]
  README.md → deploy/helm/context-fabric/Chart.yaml
- `Context Fabric Historical Reference Architecture` --references--> `ADR 0012: V1 Boundaries`  [AMBIGUOUS]
  context-fabric-stress-test.md → docs/adr/0012-v1-boundaries.md
- `main()` --calls--> `SetupLogging()`  [EXTRACTED]
  cmd/context-fabric/main.go → internal/observability/logging.go

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Decision Diagnosis and Governance Pattern** — integrations_skills_decision_diagnosis_decision_diagnosis, contextpacket, openfga, context_request_access [EXTRACTED 0.75]
- **MCP Authentication and Tool Verification Flow** — integrations_skills_connect_auth_connect_auth, mcp, rfc_9728, context_search, context_get, context_brief, context_graph, context_request_access [EXTRACTED 0.80]
- **Sparse-First Recall Routing Pattern (ADR 0015)** — integrations_skills_governed_recall_governed_recall, adr_0015, context_search, context_get, context_graph, context_brief, context_request_access [EXTRACTED 0.85]
- **Helm chart deployable module (serve/worker/migrate/bootstrap)** — deploy_helm_context_fabric_chart, deploy_helm_context_fabric_values, deploy_helm_context_fabric_templates_configmap, deploy_helm_context_fabric_templates_deployment, deploy_helm_context_fabric_templates_job, deploy_helm_context_fabric_templates_pdb, deploy_helm_context_fabric_templates_service [EXTRACTED 0.90]
- **Authorization and tenancy governance ADRs (OpenFGA, tenancy, action naming)** — docs_adr_0006_openfga_policy_and_consistency, docs_adr_0008_tenant_brand_model, docs_adr_0009_action_naming, concept_openfga_policy_provider, concept_organization_id [INFERRED 0.70]
- **Operational readiness: upgrades, SLOs, release gates** — docs_operations_upgrade_order_doc, docs_operations_slo_baselines_doc, docs_operations_release_gates [INFERRED 0.70]
- **One-image role topology roles (serve/worker/connector) and governing ADR** — concept_serve_role, concept_worker_role, concept_connector_role, docs_adr_0001_one_image_role_topology, docs_adr_0004_generic_intake_and_mapping [INFERRED 0.70]
- **AuthZ as non-overridable boundary across surfaces** — docs_threat_model_readme_doc, docs_plugin_authoring_readme_doc, integrations_skills_readme_doc, concept_openfga [INFERRED 0.75]
- **ADRs Defining V1 Scope and Governance Boundaries** — docs_adr_0010_change_feed_exposure, docs_adr_0011_connector_trust_tiers, docs_adr_0012_v1_boundaries, docs_adr_0013_knowledge_graph_first, docs_adr_0014_durable_authz_tuple_sync, docs_adr_0015_sparse_first_retrieval_and_portable_mcp [INFERRED 0.80]
- **REST/Event/AuthZ contract suite (OpenAPI, AsyncAPI, Conformance, OpenFGA tuples)** — contracts_openapi_openapi, contracts_asyncapi_asyncapi, contracts_conformance_suite, contracts_openfga_tuples_sample [INFERRED 0.80]
- **Bounded retrieval governance across skills and server** — docs_operations_retrieval_routing_doc, integrations_skills_readme_doc, integrations_skills_access_request_doc, concept_authz_batchcheck [INFERRED 0.80]
- **Deployment profile configs implementing profile schema (ADR 0002/0003)** — deploy_profiles_scaled, deploy_profiles_starter, deploy_profiles_xsama, docs_adr_0002_deployment_profiles, docs_adr_0003_bundled_vs_external_infra [INFERRED 0.80]
- **One-image deployment profiles (demo/compose/helm/coolify)** — deploy_profiles_demo, deploy_compose_docker_compose, deploy_compose_docker_compose_minimal, deploy_helm_context_fabric_values, deploy_coolify_readme [INFERRED 0.85]

## Communities (286 total, 17 thin omitted)

### Community 0 - "Server"
Cohesion: 0.06
Nodes (56): mapPacket, bufio.ReadWriter, encoding/json.RawMessage, net.Conn, net/http.Handler, net/http.HandlerFunc, net/http.PushOptions, net/http.Request (+48 more)

### Community 1 - "time.Time"
Cohesion: 0.11
Nodes (23): AuthzTupleManifest, ContentEntry, EvidenceRef, Job, Tombstone, time.Time, EnqueueForEdge(), NeedsSynchronization() (+15 more)

### Community 2 - "mapping.go"
Cohesion: 0.08
Nodes (47): allowedPredicate(), Apply(), applyTransform(), contains(), copyAttrs(), dig(), edgeFromVisibilityRef(), enforceCeilings() (+39 more)

### Community 3 - "ADR Index"
Cohesion: 0.14
Nodes (29): connector role, PolicyProvider (OpenFGA-backed), organization_id (canonical tenancy key), serve role, worker role, Context Fabric Historical Reference Architecture, OpenFGA Sample Tuples, Starter/Demo Compose Stack (+21 more)

### Community 4 - "Context Fabric Client SDK"
Cohesion: 0.08
Nodes (28): ADR-0006, RFC-9728, AccessRequestBody, BriefRequest, Citation, ClientOptions, ContextFabricClient, ContextFabricError (+20 more)

### Community 5 - "Service"
Cohesion: 0.05
Nodes (37): Durability, FeedStore, PublicEvent, github.com/jackc/pgx/v5/pgxpool.Pool, sync.RWMutex, classRank(), Index, matchFilters() (+29 more)

### Community 6 - "properties"
Cohesion: 0.09
Nodes (23): pattern, type, additionalProperties, properties, required, type, Always, IfNotPresent (+15 more)

### Community 7 - "Store"
Cohesion: 0.10
Nodes (12): coalesceTime(), firstNonEmpty(), pgx.Tx, mustJSON(), mustPgTx(), NewStore(), nullEmpty(), nullStr() (+4 more)

### Community 8 - "Memory"
Cohesion: 0.09
Nodes (23): authorizationFixture, ConsistencyPreference(), firstNonEmpty(), formatObject(), formatUser(), mapRelation(), NewFromEnv(), reason() (+15 more)

### Community 9 - "context.Context"
Cohesion: 0.08
Nodes (20): ChangeLister, ExportJobStore, ExtraStore, GraphRequest, IntakeRequest, MappingStore, memoryAuthzSeeder, QuotaStore (+12 more)

### Community 10 - "NewEventID"
Cohesion: 0.10
Nodes (27): Citation, GraphEdge, GraphNode, IsTombstoned(), NewEventID(), EdgeListOptions, Citation, ContextPacket (+19 more)

### Community 11 - "Worker"
Cohesion: 0.09
Nodes (28): jetStreamFetcher, noopBus, noopSub, outboxPayload, Worker, sync/atomic.Int64, sync.WaitGroup, RunWorker() (+20 more)

### Community 12 - "CLI Commands"
Cohesion: 0.15
Nodes (24): baseURL(), cmdAgent(), cmdConformance(), cmdDiagnose(), cmdDoctor(), cmdOps(), cmdSandbox(), cmdSource() (+16 more)

### Community 13 - "testing.T"
Cohesion: 0.09
Nodes (28): testing.T, TestListEdgesOrdersBeforeApplyingLimit(), TestBatchCheckPutsConsistencyOnRequest(), TestRequireIngestNeedsScope(), TestRequireManageDeniedWithoutScope(), app.ApplicationService, newP1Svc(), TestIntakeBatch() (+20 more)

### Community 14 - "wire"
Cohesion: 0.17
Nodes (28): app.ApplicationService, wire(), NewEvidence(), NewIndex(), NewStore(), TestTenantIsolation(), NewMemory(), changeItemCount() (+20 more)

### Community 15 - ".Intake"
Cohesion: 0.22
Nodes (18): Result, CheckReplayWindow(), defaultSpec(), firstNonEmpty(), Request, isSourceOfTruth(), preserveSoT(), VerifyHMAC() (+10 more)

### Community 16 - "enum"
Cohesion: 0.07
Nodes (31): required, enum, config, jetstream_persistence, openfga_store, pgvector, s3_object_lock, s3_versioning (+23 more)

### Community 17 - "Bounded Adaptive Retrieval Routing (ADR 0015)"
Cohesion: 0.17
Nodes (20): ADR 0015: Sparse-first recall, AuthZ can_read BatchCheck, context.brief tool, context.get tool, context.graph tool, context.request_access tool, context.search tool, OpenAPI Specification (+12 more)

### Community 18 - "Service"
Cohesion: 0.24
Nodes (9): ChangeAppender, CompletionManifest, DerivativeRef, LegalHoldChecker, Request, Logger, Service, splitEvidenceRef() (+1 more)

### Community 19 - "oidc.go"
Cohesion: 0.14
Nodes (16): jwk, jwksDoc, jwtHeader, OIDCProvider, crypto/ecdsa.PublicKey, crypto/rsa.PublicKey, audienceContains(), claimInt64() (+8 more)

### Community 20 - "suite.go"
Cohesion: 0.12
Nodes (23): Case, caseFn, Report, Result, RunOptions, Suite, HasContentFields(), New() (+15 more)

### Community 21 - "properties"
Cohesion: 0.09
Nodes (23): description, type, format, type, type, minLength, type, const (+15 more)

### Community 22 - "config.go"
Cohesion: 0.18
Nodes (19): ConnectionMode, OIDCConfig, OpenFGAConfig, PostgresConfig, S3Config, boolEnv(), defaultMode(), env() (+11 more)

### Community 23 - "properties"
Cohesion: 0.10
Nodes (21): const, type, maxLength, type, minLength, type, minLength, type (+13 more)

### Community 24 - "properties"
Cohesion: 0.07
Nodes (29): $ref, $ref, format, type, maxLength, type, maxLength, minLength (+21 more)

### Community 25 - "Store"
Cohesion: 0.05
Nodes (14): Store, Spec, ParseSpec(), ErrNotFound(), DelegationGrant, IdempotencyRecord, Organization, OutboxEntry (+6 more)

### Community 26 - "Context Packet Schema"
Cohesion: 0.08
Nodes (24): type, type, description, type, type, type, properties, audit_id (+16 more)

### Community 27 - "Helm Chart Template"
Cohesion: 0.09
Nodes (23): type, $ref, type, $ref, type, type, type, properties (+15 more)

### Community 28 - "buckets"
Cohesion: 0.12
Nodes (17): additionalProperties, properties, required, type, minLength, type, derived, quarantine (+9 more)

### Community 29 - "properties"
Cohesion: 0.12
Nodes (20): properties, audience, domain, issuer, required, type, required, type (+12 more)

### Community 30 - "properties"
Cohesion: 0.09
Nodes (36): description, type, type, type, $ref, type, $ref, default (+28 more)

### Community 31 - "TypeScript Package Config"
Cohesion: 0.09
Nodes (22): openapi-typescript, dist, README.md, src, description, devDependencies, openapi-typescript, typescript (+14 more)

### Community 32 - "context-fabric/main.go"
Cohesion: 0.14
Nodes (28): OIDCConfig, asRelWriter(), bootstrapOpenFGA(), durationEnv(), firstEnv(), firstNonEmpty(), httpServerConfigFromEnv(), intEnv() (+20 more)

### Community 33 - "replay_window_seconds"
Cohesion: 0.12
Nodes (16): type, key_id, replay_window_seconds, secret_ref, signing, default, maximum, minimum (+8 more)

### Community 34 - "Bus"
Cohesion: 0.09
Nodes (24): LedgerLogger, LedgerWriter, MemoryLogger, sync.Mutex, Connect(), ConnectConfig(), ConnectOrNoop(), credentialsFilePath() (+16 more)

### Community 35 - "required"
Cohesion: 0.17
Nodes (21): audience, buckets, database, endpoint, issuer, region, required, required (+13 more)

### Community 36 - "properties"
Cohesion: 0.17
Nodes (12): type, properties, context_space_id, resource_id, revision_id, schema_version, minLength, type (+4 more)

### Community 37 - "properties"
Cohesion: 0.14
Nodes (16): minLength, type, items, type, properties, required, host, hosts (+8 more)

### Community 38 - "chatwoot/connector.go"
Cohesion: 0.27
Nodes (13): CloudEvent, Config, Connector, decodeLooseYAML(), dig(), firstEnv(), MappingSpecJSON(), New() (+5 more)

### Community 39 - "required"
Cohesion: 0.14
Nodes (14): organization_id, retention_policy_id, source_id, status, required, allowed_record_types, authority_ceiling, classification_default (+6 more)

### Community 40 - "AgentCredential"
Cohesion: 0.18
Nodes (9): randomSecret(), NewCredentialStore(), randomSecret(), AgentCredential, AgentPrincipal, CreateAgentCredentialRequest, agentCred, CredentialStore (+1 more)

### Community 41 - "Config"
Cohesion: 0.24
Nodes (6): MCPConfig, Runtime, Secrets, Config, requireSecret(), OIDCConfig

### Community 42 - "EvidenceStore"
Cohesion: 0.50
Nodes (3): harness, EvidenceStore, evidenceBlob

### Community 43 - "Principal"
Cohesion: 0.19
Nodes (13): ApplicationService, hasScope(), isPlatformAdmin(), platformBootstrapToken(), scopesOf(), validPlatformBootstrap(), asRelationshipWriter(), principalSubject() (+5 more)

### Community 44 - "required"
Cohesion: 0.15
Nodes (13): additionalProperties, required, type, classification, observed_at, occurred_at, schema_id, schema_version (+5 more)

### Community 45 - "required"
Cohesion: 0.12
Nodes (17): classification, context_space_id, lifecycle_state, organization_id, purpose_allowlist, resource_id, resource_type, retention_policy_id (+9 more)

### Community 46 - "Graph Edge Schema"
Cohesion: 0.11
Nodes (18): maximum, minimum, type, type, type, properties, minLength, type (+10 more)

### Community 47 - "Graph Edge Sync Schema"
Cohesion: 0.11
Nodes (18): $ref, default, description, type, description, $ref, properties, description (+10 more)

### Community 48 - "Rate Quota Schema"
Cohesion: 0.11
Nodes (18): minimum, type, minimum, type, default, maximum, minimum, type (+10 more)

### Community 49 - "properties"
Cohesion: 0.11
Nodes (18): additionalProperties, properties, required, type, items, minItems, type, uniqueItems (+10 more)

### Community 50 - "$defs"
Cohesion: 0.11
Nodes (18): enum, type, $defs, connectionMode, natsDependency, openfgaDependency, postgresDependency, s3Dependency (+10 more)

### Community 51 - "properties"
Cohesion: 0.11
Nodes (18): additionalProperties, properties, required, type, const, type, network, private_dependencies (+10 more)

### Community 52 - "null"
Cohesion: 0.17
Nodes (15): type, type, type, null, string, format, type, active_mapping_spec_id (+7 more)

### Community 53 - "enum"
Cohesion: 0.14
Nodes (14): ACCEPTED, FAILED, INDEXED, PROJECTING, PURGE_PENDING, PURGED, RECLASSIFIED, TOMBSTONED (+6 more)

### Community 54 - "properties"
Cohesion: 0.15
Nodes (13): type, additionalProperties, type, type, properties, type, attributes, classification (+5 more)

### Community 55 - "required"
Cohesion: 0.11
Nodes (18): additionalProperties, description, $id, capabilities, dependencies, kind, profile, required (+10 more)

### Community 56 - "properties"
Cohesion: 0.10
Nodes (20): $ref, properties, type, type, classification, mapping_spec_id, mapping_spec_revision, retention_policy_id (+12 more)

### Community 57 - "Brief Request Schema"
Cohesion: 0.12
Nodes (17): format, type, default, type, default, description, type, default (+9 more)

### Community 58 - "properties"
Cohesion: 0.09
Nodes (23): $ref, pattern, type, minLength, type, properties, classification, content_hash (+15 more)

### Community 59 - "enum"
Cohesion: 0.15
Nodes (13): artifact, document, event, fact, message, observation, resource_type, enum (+5 more)

### Community 60 - "properties"
Cohesion: 0.10
Nodes (20): minLength, type, type, type, oidcDependency, format, type, format (+12 more)

### Community 61 - "webhookSigning"
Cohesion: 0.18
Nodes (11): minLength, type, key, secrets, webhookSigning, additionalProperties, properties, type (+3 more)

### Community 62 - "OIDC Client Config Alt"
Cohesion: 0.12
Nodes (16): minLength, type, minLength, type, type, type, minLength, type (+8 more)

### Community 63 - "Schema Migration Config"
Cohesion: 0.12
Nodes (16): const, description, type, description, enum, type, additionalProperties, properties (+8 more)

### Community 64 - "properties"
Cohesion: 0.11
Nodes (18): description, type, type, tlsConfig, type, enabled, ca_secret_ref, client_cert_secret_ref (+10 more)

### Community 65 - "Storage Capability Schema"
Cohesion: 0.12
Nodes (16): properties, description, type, description, type, description, type, jetstream_persistence (+8 more)

### Community 66 - "OIDC Claim Mapping"
Cohesion: 0.12
Nodes (16): additionalProperties, properties, type, default, type, default, type, default (+8 more)

### Community 67 - "enum"
Cohesion: 0.32
Nodes (8): enum, description, enum, type, owner, operator, platform, shared

### Community 68 - "enum"
Cohesion: 0.18
Nodes (11): ACCEPTED, FAILED, INDEXED, PROJECTING, PURGE_PENDING, PURGED, RECLASSIFIED, TOMBSTONED (+3 more)

### Community 69 - "enum"
Cohesion: 0.18
Nodes (11): enum, audit, authz_model_pin, authz_tuples, delegation_grants, events, evidence_index, graph_edges (+3 more)

### Community 70 - "properties"
Cohesion: 0.11
Nodes (18): pattern, type, $ref, pattern, type, format, type, minLength (+10 more)

### Community 71 - "Content Blob Schema"
Cohesion: 0.13
Nodes (15): minimum, type, properties, type, minLength, type, byte_size, kind (+7 more)

### Community 72 - "properties"
Cohesion: 0.12
Nodes (17): $ref, $ref, $ref, properties, brand_id, classification, context_space_id, retention_policy_id (+9 more)

### Community 73 - "properties"
Cohesion: 0.09
Nodes (22): minimum, type, type, additionalProperties, properties, required, type, sha256 (+14 more)

### Community 74 - "properties"
Cohesion: 0.15
Nodes (13): type, minLength, type, type, accessKeyKey, endpoint, pathStyle, region (+5 more)

### Community 75 - "properties"
Cohesion: 0.17
Nodes (12): type, minLength, type, type, type, properties, adminPasswordKey, database (+4 more)

### Community 76 - "NewLimiter"
Cohesion: 0.25
Nodes (9): Limiter, NewLimiter(), TestLimiterOperations(), TestLimiterRateLimits(), TestLimiterScopesSeparately(), bucket, Key, Limits (+1 more)

### Community 77 - "items"
Cohesion: 0.13
Nodes (15): items, minItems, type, items, type, uniqueItems, additionalProperties, required (+7 more)

### Community 78 - "Field Transform Schema"
Cohesion: 0.14
Nodes (14): description, minLength, type, properties, default, expr, transform, enum (+6 more)

### Community 79 - "plugin-manifest.json"
Cohesion: 0.13
Nodes (14): additionalProperties, description, $id, api_version, capabilities, id, kind, runtime (+6 more)

### Community 80 - "Plugin Version Schema"
Cohesion: 0.14
Nodes (14): const, type, format, type, type, pattern, type, properties (+6 more)

### Community 81 - "serviceAccount"
Cohesion: 0.20
Nodes (10): type, type, create, name, serviceAccount, additionalProperties, properties, required (+2 more)

### Community 82 - "properties"
Cohesion: 0.18
Nodes (11): properties, type, type, type, jetstream_persistence, openfga_store, pgvector, s3_object_lock (+3 more)

### Community 83 - "required"
Cohesion: 0.18
Nodes (11): lifecycle_state, occurred_at, organization_id, resource_id, revision_id, schema_version, required, change_type (+3 more)

### Community 84 - "properties"
Cohesion: 0.20
Nodes (10): minimum, type, minLength, type, minimum, type, properties, failureThreshold (+2 more)

### Community 85 - "properties"
Cohesion: 0.15
Nodes (14): type, minimum, type, properties, type, enabled, minAvailable, podDisruptionBudget (+6 more)

### Community 86 - "enum"
Cohesion: 0.25
Nodes (8): demo, scaled, starter, xsama, description, enum, type, profile

### Community 87 - "Plugin Capabilities Schema"
Cohesion: 0.14
Nodes (14): const, type, additionalProperties, type, const, type, properties, apiVersion (+6 more)

### Community 88 - "Graph Citation Schema"
Cohesion: 0.15
Nodes (13): items, type, items, type, $ref, items, type, citations (+5 more)

### Community 89 - "Mapping Spec Definitions"
Cohesion: 0.15
Nodes (12): additionalProperties, $defs, JsonPathOrTemplate, description, $id, additionalProperties, required, type (+4 more)

### Community 90 - "Tenant Policy Constraints"
Cohesion: 0.15
Nodes (13): const, type, const, type, const, type, const, type (+5 more)

### Community 91 - "Dry Run Edge Schema"
Cohesion: 0.15
Nodes (13): items, type, uniqueItems, description, items, type, additionalProperties, required (+5 more)

### Community 92 - "Direct Access Constraints"
Cohesion: 0.15
Nodes (13): const, type, const, type, const, type, const, type (+5 more)

### Community 93 - "taxonomy.json"
Cohesion: 0.29
Nodes (6): additionalProperties, description, $id, $schema, title, type

### Community 94 - "port"
Cohesion: 0.15
Nodes (13): maximum, minimum, type, port, service, type, properties, type (+5 more)

### Community 95 - "roles"
Cohesion: 0.15
Nodes (13): type, public_surface, roles, description, items, minItems, type, items (+5 more)

### Community 96 - "Load"
Cohesion: 0.24
Nodes (9): Profile, Load(), LoadBase(), TestLoadBaseSkipsRuntimeDeps(), TestLoadRejectsPlaceholderSecrets(), TestUseMemoryDemoWithoutDSN(), TestValidateEscapeHatchesAllowsDemo(), TestValidateEscapeHatchesRejectsOutsideDemo() (+1 more)

### Community 97 - "TypeScript Compiler Config"
Cohesion: 0.15
Nodes (12): src/**/*.ts, compilerOptions, declaration, esModuleInterop, module, moduleResolution, outDir, rootDir (+4 more)

### Community 98 - "enum"
Cohesion: 0.15
Nodes (13): items, enum, pattern, type, resource_selector, items, minItems, type (+5 more)

### Community 99 - "required"
Cohesion: 0.14
Nodes (14): owner, purpose_allowlist, runtime, status, required, actor, agent_principal, allowed_actions (+6 more)

### Community 100 - "Evidence Object Schema"
Cohesion: 0.17
Nodes (12): additionalProperties, properties, type, type, evidence, object_version, sha256, uri (+4 more)

### Community 101 - "Archive Manifest Checksums"
Cohesion: 0.17
Nodes (12): pattern, type, additionalProperties, properties, required, type, pattern, type (+4 more)

### Community 102 - "properties"
Cohesion: 0.17
Nodes (12): description, $ref, $ref, $ref, properties, $ref, brand_tag, case_type (+4 more)

### Community 103 - "capabilities"
Cohesion: 0.14
Nodes (15): description, items, minItems, type, uniqueItems, default, items, type (+7 more)

### Community 104 - "properties"
Cohesion: 0.17
Nodes (12): properties, type, type, minLength, type, properties, bundled, credentialsKey (+4 more)

### Community 105 - "properties"
Cohesion: 0.17
Nodes (12): minLength, type, properties, memory, domain, replicas, storage, minimum (+4 more)

### Community 106 - "properties"
Cohesion: 0.17
Nodes (12): properties, minLength, type, type, maximum, minimum, type, host (+4 more)

### Community 107 - "setup"
Cohesion: 0.42
Nodes (11): baseSpec(), boolPtr(), setup(), signedReq(), TestDuplicateIntakeIdempotent(), TestGeneratedObservationCannotOverwrite(), TestHMACFail(), TestIntakeEmitsParentEdgeFromVisibilityRef() (+3 more)

### Community 108 - "required"
Cohesion: 0.12
Nodes (16): additionalProperties, description, $id, organization_id, purpose, version, required, $schema (+8 more)

### Community 109 - "Citation Schema"
Cohesion: 0.18
Nodes (11): type, properties, citation_id, resource_id, revision_id, score, snippet, type (+3 more)

### Community 110 - "enum"
Cohesion: 0.29
Nodes (7): active, revoked, status, enum, type, paused, pending_verification

### Community 111 - "properties"
Cohesion: 0.22
Nodes (9): type, type, description, minLength, type, properties, causationid, correlationid (+1 more)

### Community 112 - "enum"
Cohesion: 0.25
Nodes (8): failed, status, enum, type, cancelled, completed, pending, running

### Community 113 - "enum"
Cohesion: 0.17
Nodes (12): description, items, type, uniqueItems, enum, type, source:, controlled_tag_prefixes (+4 more)

### Community 114 - "properties"
Cohesion: 0.18
Nodes (11): type, minLength, type, minLength, type, properties, apiTokenKey, apiUrl (+3 more)

### Community 115 - "required"
Cohesion: 0.12
Nodes (17): enum, type, config, dependencies, profile, serve, worker, properties (+9 more)

### Community 116 - "enum"
Cohesion: 0.22
Nodes (11): required, enum, nats, oidc, postgres, s3, enum, enum (+3 more)

### Community 117 - "required"
Cohesion: 0.18
Nodes (11): context_space_id, resource_id, resource_type, additionalProperties, required, type, mappings, content_locator (+3 more)

### Community 118 - "type"
Cohesion: 0.29
Nodes (7): items, type, type, items, type, action_restrictions, labels

### Community 119 - "Usage Budget Limits"
Cohesion: 0.20
Nodes (10): additionalProperties, properties, type, minimum, type, minimum, type, budget (+2 more)

### Community 120 - "type"
Cohesion: 0.22
Nodes (9): type, additionalProperties, description, type, boolean, null, number, string (+1 more)

### Community 121 - "type"
Cohesion: 0.25
Nodes (9): description, type, null, object, string, brand_id, supersedes_revision_id, type (+1 more)

### Community 122 - "enum"
Cohesion: 0.14
Nodes (14): items, minItems, type, uniqueItems, items, enum, type, artifact (+6 more)

### Community 123 - "properties"
Cohesion: 0.20
Nodes (10): type, additionalProperties, type, type, additionalProperties, properties, type, annotations (+2 more)

### Community 124 - "database"
Cohesion: 0.67
Nodes (3): minLength, type, database

### Community 125 - "metadata"
Cohesion: 0.12
Nodes (16): type, version, additionalProperties, properties, required, type, maxLength, minLength (+8 more)

### Community 126 - "jetstream"
Cohesion: 0.14
Nodes (14): enum, additionalProperties, description, enum, required, type, domain, jetstream (+6 more)

### Community 127 - "Change Event Schema"
Cohesion: 0.22
Nodes (8): additionalProperties, description, $id, not, anyOf, $schema, title, type

### Community 128 - "Resource Change Events"
Cohesion: 0.22
Nodes (9): enum, type, change_type, resource.accepted, resource.failed, resource.indexed, resource.purged, resource.reclassified (+1 more)

### Community 129 - "enum"
Cohesion: 0.22
Nodes (10): enum, account_management, agent_assist, finance, marketing, support, enum, engineering (+2 more)

### Community 130 - "tombstone"
Cohesion: 0.18
Nodes (11): const, type, dominates, reason, requested_at, tombstone, type, format (+3 more)

### Community 131 - "visibility_mapping_policy"
Cohesion: 0.11
Nodes (18): type, uniqueItems, type, enum, type, allowed_visibility_refs, default_visibility_ref, mode (+10 more)

### Community 132 - "properties"
Cohesion: 0.12
Nodes (17): additionalProperties, properties, required, schema_id, version, type, type, mapping_spec_id (+9 more)

### Community 133 - "Health Probes Schema"
Cohesion: 0.22
Nodes (9): $ref, properties, type, liveness, probes, readiness, startup, $ref (+1 more)

### Community 134 - "properties"
Cohesion: 0.22
Nodes (9): properties, $ref, $ref, $ref, nats, oidc, postgres, s3 (+1 more)

### Community 135 - "Auth Credential Tests"
Cohesion: 0.39
Nodes (7): NewCredentialStore(), TestRevokeIsOrgScoped(), TestChainedAPIKeyBearerAuthenticates(), TestChainedAPIKeyHeaderField(), TestChainedLocalTokenStillWorks(), TestChainedRejectsUnknownKey(), WithAPIKeys()

### Community 136 - "required"
Cohesion: 0.25
Nodes (8): openfga, additionalProperties, required, type, persistence, evidence, jetstream, ledger

### Community 137 - "required"
Cohesion: 0.22
Nodes (9): additionalProperties, required, type, jetstream_persistence, openfga_store, pgvector, s3_object_lock, s3_versioning (+1 more)

### Community 138 - "NewOIDC"
Cohesion: 0.47
Nodes (9): crypto/rsa.PrivateKey, net/http/httptest.Server, NewOIDC(), signRS256(), testJWKS(), TestOIDCAuthenticateScopesAndOrg(), TestOIDCAuthenticateScpClaim(), TestOIDCMissingOrgStillAuthenticates() (+1 more)

### Community 139 - "Redaction"
Cohesion: 0.14
Nodes (14): Redaction, items, type, profile, type, fields, profile, reason_code (+6 more)

### Community 140 - "error"
Cohesion: 0.22
Nodes (9): additionalProperties, properties, type, object, type, error, message, reason_code (+1 more)

### Community 141 - "Authority Constraints Schema"
Cohesion: 0.25
Nodes (8): additionalProperties, default, type, authority_ceiling_from_source, cannot_broaden_acl, cannot_mint_organization, client_fields_may_only_narrow, constraints

### Community 142 - "timestamps"
Cohesion: 0.18
Nodes (11): observed_at, occurred_at, $ref, $ref, observed_at, occurred_at, timestamps, additionalProperties (+3 more)

### Community 143 - "Forbidden Direct Access"
Cohesion: 0.25
Nodes (8): broad_evidence_credentials, direct_database, direct_nats, direct_openfga, additionalProperties, default, type, forbidden

### Community 144 - "Emitted Event Types"
Cohesion: 0.25
Nodes (8): items, minItems, type, uniqueItems, enum, emits, context.ingest.request.v1, context.lifecycle.notification.v1

### Community 145 - "Tags Schema"
Cohesion: 0.25
Nodes (8): maxLength, minLength, type, tags, description, items, type, uniqueItems

### Community 146 - "enum"
Cohesion: 0.33
Nodes (6): failed, never, enum, type, last_status, ok

### Community 147 - "Classification Levels Enum"
Cohesion: 0.25
Nodes (8): description, enum, type, classification, confidential, internal, public, restricted

### Community 148 - "download_uri"
Cohesion: 0.25
Nodes (9): description, format, type, format, type, null, string, download_uri (+1 more)

### Community 149 - "logging.go"
Cohesion: 0.20
Nodes (11): log/slog.Attr, log/slog.Handler, log/slog.Level, log/slog.Logger, log/slog.Record, firstNonEmptyEnv(), looksSensitive(), parseLogLevel() (+3 more)

### Community 150 - "Prediction Scoring Tool"
Cohesion: 0.46
Nodes (7): loadPreds(), loadTasks(), main(), percentile(), setOf(), prediction, task

### Community 151 - "types.go"
Cohesion: 0.11
Nodes (13): sanitize(), SpecFromPorts(), AuditEvent, InboxEntry, IntakeEvent, MappingSpec, PolicyEval, PolicyResult (+5 more)

### Community 152 - "OpenAPI Generation Script"
Cohesion: 0.25
Nodes (7): bin, here, openapi, outDir, outFile, r, root

### Community 153 - "$defs"
Cohesion: 0.11
Nodes (19): additionalProperties, required, type, $defs, Citation, GraphEdge, GraphNode, additionalProperties (+11 more)

### Community 154 - "type"
Cohesion: 0.28
Nodes (9): type, boolean, null, number, string, reviewed_by, type, type (+1 more)

### Community 155 - "Delegation Grant Schema"
Cohesion: 0.29
Nodes (6): additionalProperties, description, $id, $schema, title, type

### Community 156 - "Event Envelope Schema"
Cohesion: 0.29
Nodes (6): additionalProperties, description, $id, $schema, title, type

### Community 157 - "required"
Cohesion: 0.25
Nodes (8): id, source, required, data, datacontenttype, specversion, time, type

### Community 158 - "properties"
Cohesion: 0.09
Nodes (22): type, additionalProperties, properties, required, type, Always, IfNotPresent, Never (+14 more)

### Community 159 - "export-manifest.json"
Cohesion: 0.13
Nodes (14): additionalProperties, description, $id, organization_id, status, required, $schema, title (+6 more)

### Community 160 - "ValidateWebhookTarget"
Cohesion: 0.33
Nodes (7): net.IP, isBlockedHost(), isBlockedIP(), TestValidateWebhookTargetBlocksLoopback(), TestValidateWebhookTargetBlocksMetadata(), TestValidateWebhookTargetHTTPSRequired(), ValidateWebhookTarget()

### Community 161 - "EvidenceRouter"
Cohesion: 0.09
Nodes (22): newS3Evidence(), Config, github.com/aws/aws-sdk-go-v2/service/s3.Client, github.com/aws/aws-sdk-go-v2/service/s3.PresignClient, io.ReadCloser, io.Reader, evidenceTier(), NewRouter() (+14 more)

### Community 162 - "enum"
Cohesion: 0.29
Nodes (7): source, enum, type, kind, enricher, parser, transform

### Community 163 - "OpenAPI Contract"
Cohesion: 0.50
Nodes (5): AsyncAPI Contract, Conformance Suite, OpenAPI Contract, Compatibility Matrix, CI Workflow

### Community 164 - "enum"
Cohesion: 0.25
Nodes (8): account_management, agent_assist, finance, marketing, support, purpose, enum, type

### Community 165 - "Review State Enum"
Cohesion: 0.29
Nodes (7): review_state, enum, type, approved, needs_review, rejected, unreviewed

### Community 166 - "Source Registration Schema"
Cohesion: 0.29
Nodes (6): additionalProperties, description, $id, $schema, title, type

### Community 167 - "Retention Policy Enum"
Cohesion: 0.29
Nodes (7): retention, enum, type, 24m, 30d, indefinite_policy, legal_hold

### Community 168 - "Trust Level Enum"
Cohesion: 0.29
Nodes (7): trust, enum, type, generated, trusted_internal, trusted_system, untrusted_external

### Community 169 - "networkPolicy"
Cohesion: 0.11
Nodes (19): type, minimum, type, jobSpec, required, properties, required, type (+11 more)

### Community 170 - "required"
Cohesion: 0.25
Nodes (8): api_version, id, organization_id, source_id, status, required, mappings, revision

### Community 171 - "Runbook: Restore"
Cohesion: 0.15
Nodes (11): Data integrity checks, Diagnosis, Remediation, Runbook: Postgres recovery, Symptoms, Preconditions, Procedure, RTO target (+3 more)

### Community 172 - "Deploy Context Fabric — starter Compose (production)"
Cohesion: 0.22
Nodes (9): Architecture, CI parity, Deploy Context Fabric — starter Compose (production), Image promotion, Operations, Prerequisites, Quick start, Related (+1 more)

### Community 173 - "ports.go"
Cohesion: 0.11
Nodes (14): AgentClaims, APIKeyResolver, Chained, NewAPIKeyResolver(), looksLikeJWT(), AuthzTupleOp, CredentialProvider, EventBus (+6 more)

### Community 174 - "ADR 0013: Knowledge-Graph-First"
Cohesion: 0.30
Nodes (9): ADR 0010: Change Feed and Webhook Exposure, ADR 0013: Knowledge-Graph-First, ADR 0014: Durable AuthZ Tuple Synchronization, Backup and Restore, Degraded Mode, Diagnostics, Release Gates (CI), Retrieval Evaluation Corpus (FTS + graph gate) (+1 more)

### Community 175 - "required"
Cohesion: 0.29
Nodes (7): classification, domain, purpose, trust, required, authority, retention

### Community 176 - "Purpose Allowlist Schema"
Cohesion: 0.33
Nodes (6): $ref, purpose_allowlist, items, minItems, type, uniqueItems

### Community 177 - "Send Policy Enum"
Cohesion: 0.33
Nodes (6): send_policy, default, enum, type, denied, human_approval_required

### Community 178 - "enum"
Cohesion: 0.29
Nodes (7): active, status, enum, type, deprecated, draft, reviewed

### Community 179 - "Trust Tier Enum"
Cohesion: 0.33
Nodes (6): trust_tier, enum, type, deterministic_transform, first_party_connector, third_party_plugin

### Community 180 - "Purpose Allowlist Schema"
Cohesion: 0.33
Nodes (6): $ref, purpose_allowlist, items, minItems, type, uniqueItems

### Community 181 - "required"
Cohesion: 0.29
Nodes (7): properties, required, type, derived, quarantine, raw, buckets

### Community 182 - "properties"
Cohesion: 0.22
Nodes (9): properties, type, minLength, type, minLength, type, indexBackend, listenAddr (+1 more)

### Community 183 - "values.schema.json"
Cohesion: 0.13
Nodes (14): additionalProperties, $defs, probe, roleWorkload, $id, path, required, type (+6 more)

### Community 184 - "connectionMode"
Cohesion: 0.40
Nodes (5): enum, type, bundled, external, connectionMode

### Community 185 - "Runbook: upgrade and rollback"
Cohesion: 0.22
Nodes (9): Helm (scaled profile), Post-upgrade verification, Pre-flight, Related, Rollback, Runbook: upgrade and rollback, Upgrade (Compose starter), Upgrade steps (+1 more)

### Community 186 - "enum"
Cohesion: 0.29
Nodes (7): demo, scaled, starter, xsama, enum, type, profile

### Community 187 - "record.json"
Cohesion: 0.29
Nodes (6): additionalProperties, description, $id, $schema, title, type

### Community 188 - "required"
Cohesion: 0.25
Nodes (8): additionalProperties, required, type, capabilities, config, listenAddr, logLevel, openfgaModelId

### Community 189 - "migrate.go"
Cohesion: 0.30
Nodes (11): runBootstrap(), runMigrate(), AdminDSN(), Bootstrap(), CheckApplied(), loadEmbedded(), loadMigrations(), MigrationsDir() (+3 more)

### Community 190 - "Purpose Allowlist Schema"
Cohesion: 0.40
Nodes (5): $ref, purpose_allowlist, items, type, uniqueItems

### Community 191 - "Tags List Schema"
Cohesion: 0.40
Nodes (5): type, tags, items, type, uniqueItems

### Community 192 - "Example ID Pattern"
Cohesion: 0.40
Nodes (5): examples, pattern, type, id, com.example.chatwoot-source

### Community 193 - "type"
Cohesion: 0.40
Nodes (6): null, string, sbom_attestation_ref, signature_ref, type, type

### Community 194 - "Runtime Enum"
Cohesion: 0.40
Nodes (5): runtime, enum, type, oci, wasm

### Community 195 - "Evidence Provenance Enum"
Cohesion: 0.40
Nodes (5): enum, corroborating, inferred, source_of_truth, user_claim

### Community 196 - "properties"
Cohesion: 0.29
Nodes (7): type, type, properties, authority, domain, purpose, type

### Community 197 - "required"
Cohesion: 0.25
Nodes (8): required, enum, memory, nats, oidc, openfga, postgres, s3

### Community 198 - "enum"
Cohesion: 0.29
Nodes (7): enum, type, logLevel, debug, error, info, warn

### Community 199 - "Pagination Cursor Schema"
Cohesion: 0.50
Nodes (4): description, minLength, type, cursor

### Community 200 - "Event ID Schema Field"
Cohesion: 0.50
Nodes (4): description, minLength, type, event_id

### Community 201 - "Profile Version Schema Field"
Cohesion: 0.50
Nodes (4): description, minLength, type, profile_version

### Community 202 - "Actor Schema Field"
Cohesion: 0.50
Nodes (4): description, pattern, type, actor

### Community 203 - "revoked_at"
Cohesion: 0.40
Nodes (5): null, string, revoked_at, format, type

### Community 204 - "Owner Schema Field"
Cohesion: 0.50
Nodes (4): description, pattern, type, owner

### Community 205 - "Runtime Schema Field"
Cohesion: 0.50
Nodes (4): runtime, description, pattern, type

### Community 206 - "Subject Schema Field"
Cohesion: 0.50
Nodes (4): subject, description, pattern, type

### Community 207 - "ID Schema Field"
Cohesion: 0.50
Nodes (4): description, minLength, type, id

### Community 208 - "Idempotency Key Schema Field"
Cohesion: 0.50
Nodes (4): maxLength, minLength, type, idempotency_key

### Community 209 - "type"
Cohesion: 0.40
Nodes (6): type, null, string, brand_id, source_id, type

### Community 210 - "Source Schema Field"
Cohesion: 0.50
Nodes (4): source, description, minLength, type

### Community 211 - "Confidence Score Schema Field"
Cohesion: 0.50
Nodes (4): maximum, minimum, type, confidence

### Community 212 - "Organization ID Schema Field"
Cohesion: 0.50
Nodes (4): description, minLength, type, organization_id

### Community 213 - "Resource ID Schema Field"
Cohesion: 0.50
Nodes (4): resource_id, description, minLength, type

### Community 214 - "Revision ID Schema Field"
Cohesion: 0.50
Nodes (4): revision_id, description, minLength, type

### Community 215 - "enum"
Cohesion: 0.33
Nodes (6): active, revoked, status, enum, type, expired

### Community 216 - "visibility_ref"
Cohesion: 0.50
Nodes (4): visibility_ref, description, minLength, type

### Community 217 - "synthetic/connector.go"
Cohesion: 0.57
Nodes (5): env(), New(), RunCLI(), Config, Connector

### Community 218 - "required"
Cohesion: 0.33
Nodes (6): buckets, endpoint, region, s3, required, type

### Community 219 - "Runbook: backup and restore"
Cohesion: 0.29
Nodes (7): Backup procedure, Quarterly drill, Related, Restore procedure, RPO / RTO targets (starter profile), Runbook: backup and restore, What is captured

### Community 220 - "Model ID Schema Field"
Cohesion: 0.50
Nodes (4): description, minLength, type, model_id

### Community 221 - "properties"
Cohesion: 0.29
Nodes (7): description, type, description, type, properties, evidence, ledger

### Community 222 - "Created At Schema Field"
Cohesion: 0.67
Nodes (3): format, type, created_at

### Community 223 - "allowed_actions"
Cohesion: 0.40
Nodes (5): description, minItems, type, uniqueItems, allowed_actions

### Community 224 - "source_revision"
Cohesion: 0.67
Nodes (3): source_revision, minLength, type

### Community 225 - "Content Locator Schema Field"
Cohesion: 0.67
Nodes (3): description, type, content_locator

### Community 226 - "Context Space ID Schema Field"
Cohesion: 0.67
Nodes (3): minLength, type, contextspaceid

### Community 227 - "Data Content Type Schema Field"
Cohesion: 0.67
Nodes (3): const, type, datacontenttype

### Community 228 - "Data Schema Field"
Cohesion: 0.67
Nodes (3): format, type, dataschema

### Community 229 - "Observed At Schema Field"
Cohesion: 0.67
Nodes (3): format, type, observed_at

### Community 230 - "Occurred At Schema Field"
Cohesion: 0.67
Nodes (3): format, type, occurred_at

### Community 231 - "Producer Schema Field"
Cohesion: 0.67
Nodes (3): minLength, type, producer

### Community 232 - "Resource ID Schema Field"
Cohesion: 0.67
Nodes (3): resourceid, minLength, type

### Community 233 - "Revision ID Schema Field"
Cohesion: 0.67
Nodes (3): revisionid, minLength, type

### Community 234 - "Schema ID Schema Field"
Cohesion: 0.67
Nodes (3): schema_id, minLength, type

### Community 235 - "secrets"
Cohesion: 0.50
Nodes (4): secrets, description, type, uniqueItems

### Community 236 - "Spec Version Schema Field"
Cohesion: 0.67
Nodes (3): specversion, const, type

### Community 237 - "Time Schema Field"
Cohesion: 0.67
Nodes (3): time, format, type

### Community 238 - "Type Pattern Schema Field"
Cohesion: 0.67
Nodes (3): type, pattern, type

### Community 240 - "system_time"
Cohesion: 0.50
Nodes (4): system_time, description, format, type

### Community 241 - "Parent Resource ID Schema Field"
Cohesion: 0.67
Nodes (3): description, $ref, parent_resource_id

### Community 242 - "Visibility Ref Schema Field"
Cohesion: 0.67
Nodes (3): visibility_ref, description, $ref

### Community 244 - "occurred_at"
Cohesion: 0.67
Nodes (3): format, type, occurred_at

### Community 245 - "Retention Policy ID Schema Field"
Cohesion: 0.67
Nodes (3): retention_policy_id, minLength, type

### Community 246 - "organization_id"
Cohesion: 0.67
Nodes (3): minLength, type, organization_id

### Community 247 - "AsAPIError"
Cohesion: 0.20
Nodes (10): TestLocalAgentRole(), TestLocalAuthenticate(), TestLocalDiscover(), TestLocalRejectsMalformed(), toolErrorResult(), AsAPIError(), TestRequireOrgEmptyPath(), TestRequireOrgMismatch() (+2 more)

### Community 248 - "grant_id"
Cohesion: 0.67
Nodes (3): minLength, type, grant_id

### Community 249 - "Title Schema Field"
Cohesion: 0.67
Nodes (3): title, maxLength, type

### Community 250 - "subject"
Cohesion: 0.67
Nodes (3): subject, description, type

### Community 251 - "traceparent"
Cohesion: 0.67
Nodes (3): traceparent, description, type

### Community 252 - "content_locator"
Cohesion: 0.67
Nodes (3): description, type, content_locator

### Community 253 - "source_event_id"
Cohesion: 0.67
Nodes (3): source_event_id, minLength, type

### Community 257 - "enum"
Cohesion: 0.33
Nodes (6): enum, type, method, ed25519, hmac_sha256, oidc_jwt

### Community 258 - "dependencies"
Cohesion: 0.50
Nodes (4): additionalProperties, type, type, dependencies

### Community 264 - "required"
Cohesion: 0.33
Nodes (6): database, host, port, required, type, postgres

### Community 267 - "endpoint"
Cohesion: 0.33
Nodes (6): endpoint, additionalProperties, required, type, host, port

### Community 268 - "Helm values.yaml (scaled profile)"
Cohesion: 0.43
Nodes (7): Helm Chart.yaml, Helm ConfigMap Template, Helm Deployment Template (serve/worker), Helm Job Template (migrate/bootstrap), Helm PodDisruptionBudget Template, Helm Service Template, Helm values.yaml (scaled profile)

### Community 269 - "ADR 0016: Compose-first production release"
Cohesion: 0.33
Nodes (6): ADR 0016: Compose-first production release, Consequences, Context, Decision, Related, Status

### Community 270 - "Runbook: AuthZ outbox backlog"
Cohesion: 0.33
Nodes (5): Diagnosis, Prevention, Remediation, Runbook: AuthZ outbox backlog, Symptoms

### Community 271 - "Runbook: Backup failure"
Cohesion: 0.33
Nodes (5): Diagnosis, Remediation, RPO target, Runbook: Backup failure, Symptoms

### Community 272 - "Runbook: OpenFGA outage"
Cohesion: 0.33
Nodes (5): Diagnosis, Fail-closed behavior, Remediation, Runbook: OpenFGA outage, Symptoms

### Community 273 - "Runbook: `/health/ready` returns false"
Cohesion: 0.33
Nodes (6): Common causes, Escalation, Immediate checks, Related, Runbook: `/health/ready` returns false, Symptoms

### Community 274 - "Context Fabric Threat Model (STRIDE)"
Cohesion: 0.40
Nodes (5): OpenFGA Authorization Model, Upgrade Order, Connector Conformance Suite, Plugin Authoring Guide, Context Fabric Threat Model (STRIDE)

### Community 275 - "openfga"
Cohesion: 0.33
Nodes (6): description, enum, $ref, type, openfga, store_and_tuples

### Community 276 - "Skill: decision-diagnosis"
Cohesion: 0.67
Nodes (4): ContextPacket, Skill: decision-diagnosis, Chatwoot MappingSpec (map_chatwoot_messages), OpenFGA authorization system

### Community 277 - "dependencies"
Cohesion: 0.50
Nodes (4): additionalProperties, type, type, dependencies

### Community 278 - "upgrade.sh"
Cohesion: 0.67
Nodes (3): GONTEXT_IMAGE, upgrade.sh script, usage()

### Community 279 - "controlled_taxonomy"
Cohesion: 0.67
Nodes (3): additionalProperties, type, controlled_taxonomy

### Community 280 - "role_secret_ref"
Cohesion: 0.67
Nodes (3): role_secret_ref, description, type

## Ambiguous Edges - Review These
- `Context Fabric Historical Reference Architecture` → `ADR 0012: V1 Boundaries`  [AMBIGUOUS]
  context-fabric-stress-test.md · relation: references

## Knowledge Gaps
- **1452 isolated node(s):** `$schema`, `$id`, `title`, `description`, `type` (+1447 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 1521 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **17 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **What is the exact relationship between `Context Fabric Historical Reference Architecture` and `ADR 0012: V1 Boundaries`?**
  _Edge tagged AMBIGUOUS (relation: references) - confidence is low._
- **Why does `wire()` connect `wire` to `context-fabric/main.go`, `EvidenceRouter`, `Bus`, `Server`, `Service`, `Auth Credential Tests`, `Memory`, `AgentCredential`, `Store`, `NewOIDC`, `Config`, `ports.go`, `NewLimiter`, `Service`, `suite.go`, `migrate.go`?**
  _High betweenness centrality (0.012) - this node is a cross-community bridge._
- **Why does `ApplicationService` connect `context.Context` to `Server`, `time.Time`, `Service`, `AgentCredential`, `EvidenceStore`, `Principal`, `NewLimiter`, `ports.go`, `NewEventID`, `Service`, `Store`?**
  _High betweenness centrality (0.009) - this node is a cross-community bridge._
- **Why does `properties` connect `Helm Chart Template` to `dependencies`, `networkPolicy`, `serviceAccount`, `required`, `values.schema.json`, `enum`, `properties`, `required`, `webhookSigning`, `properties`?**
  _High betweenness centrality (0.009) - this node is a cross-community bridge._
- **What connects `$schema`, `$id`, `title` to the rest of the system?**
  _1452 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Server` be split into smaller, more focused modules?**
  _Cohesion score 0.058403361344537816 - nodes in this community are weakly interconnected._
- **Should `time.Time` be split into smaller, more focused modules?**
  _Cohesion score 0.10526315789473684 - nodes in this community are weakly interconnected._