# Graph Report - gontext  (2026-09-01)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 3215 nodes · 5725 edges · 264 communities (249 shown, 15 thin omitted)
- Extraction: 99% EXTRACTED · 1% INFERRED · 0% AMBIGUOUS · INFERRED: 63 edges (avg confidence: 0.77)
- Token cost: 11,101 input · 7,474 output

## Graph Freshness
- Built from commit: `eded47a3`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- HTTP JSON Handler
- Authz Ledger Store
- Attribute Mapping Logic
- Policy & Deployment Docs
- Context Fabric Client SDK
- Webhook Delivery Store
- Container Image Schema
- Postgres Store Helpers
- OpenFGA Consistency Client
- Application Service Layer
- Graph Metrics & Nodes
- JWKS OIDC Auth
- CLI Commands
- Authz Service Tests
- Tenant/Authz Test Suite
- Event Bus Audit Log
- Service Dependency Schema
- Context Fabric API Tools
- Index Search Filters
- Authz Worker Reconciliation
- Test Case Reporting
- Export Job Schema
- Resource Store Interfaces
- Mapping Spec Schema
- Source Config Schema
- Organization Store Records
- Context Packet Schema
- Helm Chart Template
- Ingest Bucket Schema
- Service Config Schema
- Backup Store Config
- TypeScript Package Config
- Main Bootstrap Config
- Webhook Signing Config
- NATS Event Bus
- Storage Config Schema
- Change Event Schema
- Export Evidence Bundle
- API Key Auth Resolver
- Source Registration Schema
- Agent Credential Store
- Audit Ledger Signing
- App Config Loader
- S3 Evidence Store
- Change Event Metadata
- Resource Provenance Schema
- Graph Edge Schema
- Graph Edge Sync Schema
- Rate Quota Schema
- Backup Policy Schema
- Dependency Config Schema
- Network Serve Config
- Mapping Spec Status
- Record Lifecycle States
- Graph Node Schema
- Deployment Profile Config
- Record Classification Schema
- Brief Request Schema
- Evidence Provenance Schema
- Resource Type Enum
- OIDC Client Config
- Intake Ingest Service
- OIDC Client Config Alt
- Schema Migration Config
- TLS Connection Config
- Storage Capability Schema
- OIDC Claim Mapping
- Local Auth Tests
- Chatwoot Connector
- Domain Resource Enum
- Delegation Grant Schema
- Content Blob Schema
- Context Space Schema
- Content Object Schema
- S3 Storage Config
- Database Credentials Config
- Rate Limiter
- Content Bundle Schema
- Field Transform Schema
- Plugin Manifest Schema
- Plugin Version Schema
- K8s Service Account
- Storage Feature Flags
- Server Runtime Config
- Health Probe Config
- Pod Scaling Config
- Demo Deployment Profile
- Plugin Capabilities Schema
- Graph Citation Schema
- Mapping Spec Definitions
- Tenant Policy Constraints
- Dry Run Edge Schema
- Direct Access Constraints
- Classification Taxonomy
- K8s Service Ports
- Public Surface Roles
- API Error Types
- TypeScript Compiler Config
- Context Action Enum
- Access Grant Schema
- Evidence Object Schema
- Archive Manifest Checksums
- Case Classification Tags
- Egress and Secrets Config
- Credential Config Schema
- Deployment Resource Config
- Endpoint Schema
- Ingest Idempotency Tests
- Context Packet Schema
- Citation Schema
- Verification Status Enum
- Event Metadata Fields
- Job Status Enum
- Controlled Tag Prefixes
- API Credential Config
- Service Network Surface
- Required Dependencies Enum
- Database Migration Runner
- Action Restrictions Schema
- Usage Budget Limits
- Attribute Type Schema
- tombstone
- Allowed Record Types
- Job Workload Spec
- Database Secret Refs
- Name Version Schema
- Jetstream Storage Config
- Change Event Schema
- Resource Change Events
- Purpose Classification Enum
- Dominance Decision Schema
- Visibility Mapping Policy
- Mapping Spec Schema
- Health Probes Schema
- Service Dependency Refs
- Auth Credential Tests
- Graph Edge Operations
- Synthetic Connector Runner
- Authorization Fixture Tests
- Redaction Schema
- Error Response Schema
- Authority Constraints Schema
- Timestamp Fields Schema
- Forbidden Direct Access
- Emitted Event Types
- Tags Schema
- Verification Status Schema
- Classification Levels Enum
- Service Config Schema
- Ownership Enum
- Prediction Scoring Tool
- Source Registration Trust
- OpenAPI Generation Script
- Graph Edge Schema
- Audit Authorization Record
- Delegation Grant Schema
- Event Envelope Schema
- CloudEvents Envelope Schema
- Schema Version List
- Export Manifest Schema
- Export Manifest Status
- Capabilities Schema
- Pipeline Stage Kind
- Record Schema
- Evidence Object Schema
- Review State Enum
- Source Registration Schema
- Retention Policy Enum
- Trust Level Enum
- Job Backoff Config
- Config Schema
- Evidence Ledger Schema
- Persistence Ledger Config
- ID Generation Utility
- OpenFGA Bootstrap
- Resource Selector Schema
- Purpose Allowlist Schema
- Send Policy Enum
- Document Status Enum
- Trust Tier Enum
- Purpose Allowlist Schema
- Visibility Mapping Policy
- Domain Enum
- Values Schema
- Connection Mode Enum
- Endpoint Schema
- Failure Behavior Enum
- Metadata Schema
- OpenFGA Store Schema
- Local Auth Provider
- Purpose Allowlist Schema
- Tags List Schema
- Example ID Pattern
- SBOM Signature Refs
- Runtime Enum
- Evidence Provenance Enum
- Authority Purpose Schema
- Required Services List
- Policy Evaluation Logic
- Pagination Cursor Schema
- Event ID Schema Field
- Profile Version Schema Field
- Actor Schema Field
- Revoked At Schema Field
- Owner Schema Field
- Runtime Schema Field
- Subject Schema Field
- ID Schema Field
- Idempotency Key Schema Field
- Organization ID Schema Field
- Source Schema Field
- Confidence Score Schema Field
- Organization ID Schema Field
- Resource ID Schema Field
- Revision ID Schema Field
- Valid Time Schema Field
- Visibility Ref Schema Field
- Display Name Schema Field
- Dependencies Schema Field
- Dependencies Schema Field
- Model ID Schema Field
- S3 Store Config Code
- Created At Schema Field
- Created By Schema Field
- Expires At Schema Field
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
- Schema Version Schema Field
- Spec Version Schema Field
- Time Schema Field
- Type Pattern Schema Field
- Controlled Taxonomy Schema Field
- Description Schema Field
- Parent Resource ID Schema Field
- Visibility Ref Schema Field
- Content Hash Schema Field
- Context Space ID Schema Field
- Retention Policy ID Schema Field
- Source External ID Schema Field
- Source Revision Schema Field
- Source System Schema Field
- Title Schema Field
- Path Style Schema Field
- Region Schema Field
- Source Authority Schema Ref
- Classification Schema Ref
- Resource ID Schema Ref
- Resource Type Schema Ref
- Retention Policy Schema Ref
- Source Authority Schema Ref
- Source External ID Schema Ref
- Backup Script
- Reconcile Script
- Restore Script
- Secrets Schema Definition
- Context Fabric Repository

## God Nodes (most connected - your core abstractions)
1. `Store` - 74 edges
2. `ApplicationService` - 65 edges
3. `Server` - 54 edges
4. `Store` - 54 edges
5. `ErrNotFound()` - 51 edges
6. `ErrValidation()` - 48 edges
7. `Credentials` - 46 edges
8. `writeErr()` - 44 edges
9. `writeJSON()` - 43 edges
10. `bearerCreds()` - 38 edges

## Surprising Connections (you probably didn't know these)
- `ADR 0004: Generic Intake and MappingSpec` --references--> `mapping_specs`  [EXTRACTED]
  docs/adr/0004-generic-intake-and-mapping.md → contracts/jsonschema/export-manifest.json
- `Context Fabric Historical Reference Architecture` --references--> `ADR 0012: V1 Boundaries`  [AMBIGUOUS]
  context-fabric-stress-test.md → docs/adr/0012-v1-boundaries.md
- `wire()` --references--> `Server`  [EXTRACTED]
  cmd/context-fabric/main.go → internal/httpapi/server.go
- `wire()` --calls--> `New()`  [EXTRACTED]
  cmd/context-fabric/main.go → internal/httpapi/server.go
- `wire()` --calls--> `NewOIDC()`  [EXTRACTED]
  cmd/context-fabric/main.go → internal/authn/oidc.go

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

## Communities (264 total, 15 thin omitted)

### Community 0 - "HTTP JSON Handler"
Cohesion: 0.08
Nodes (43): mapPacket, encoding/json.RawMessage, net/http.Handler, net/http.HandlerFunc, net/http.Request, net/http.ResponseWriter, net/http.ServeMux, callMCPSearch() (+35 more)

### Community 1 - "Authz Ledger Store"
Cohesion: 0.07
Nodes (14): context.Context, Store, Store, AuthzTupleOp, ChangeEvent, DelegationGrant, OutboxEntry, Quota (+6 more)

### Community 2 - "Attribute Mapping Logic"
Cohesion: 0.10
Nodes (44): allowedPredicate(), Apply(), applyTransform(), contains(), copyAttrs(), dig(), edgeFromVisibilityRef(), enforceCeilings() (+36 more)

### Community 3 - "Policy & Deployment Docs"
Cohesion: 0.08
Nodes (46): PolicyProvider (OpenFGA-backed), organization_id (canonical tenancy key), serve role, worker role, Context Fabric Historical Reference Architecture, AsyncAPI Contract, Conformance Suite, OpenAPI Contract (+38 more)

### Community 4 - "Context Fabric Client SDK"
Cohesion: 0.08
Nodes (28): ADR-0006, RFC-9728, AccessRequestBody, BriefRequest, Citation, ClientOptions, ContextFabricClient, ContextFabricError (+20 more)

### Community 5 - "Webhook Delivery Store"
Cohesion: 0.12
Nodes (14): Durability, FeedStore, PublicEvent, time.Duration, deliveredAt(), NewWebhookStore(), eventMatches(), DeliveryAttempt (+6 more)

### Community 6 - "Container Image Schema"
Cohesion: 0.05
Nodes (43): type, additionalProperties, properties, required, type, Always, Never, tag (+35 more)

### Community 7 - "Postgres Store Helpers"
Cohesion: 0.12
Nodes (13): github.com/jackc/pgx/v5/pgxpool.Pool, coalesceTime(), firstNonEmpty(), pgx.Tx, mustJSON(), mustPgTx(), NewStore(), nullEmpty() (+5 more)

### Community 8 - "OpenFGA Consistency Client"
Cohesion: 0.11
Nodes (17): ConsistencyPreference(), firstNonEmpty(), formatObject(), formatUser(), mapRelation(), NewFromEnv(), reason(), TestConsistencyPreferenceMapping() (+9 more)

### Community 9 - "Application Service Layer"
Cohesion: 0.13
Nodes (5): ApplicationService, VersionInfo, redactSource(), RequireOrg(), Credentials

### Community 10 - "Graph Metrics & Nodes"
Cohesion: 0.12
Nodes (29): Citation, GraphEdge, GraphNode, Add(), Get(), Handler(), Inc(), SetGauge() (+21 more)

### Community 11 - "JWKS OIDC Auth"
Cohesion: 0.12
Nodes (25): jwk, jwksDoc, jwtHeader, OIDCConfig, OIDCProvider, crypto/ecdsa.PublicKey, crypto/rsa.PrivateKey, crypto/rsa.PublicKey (+17 more)

### Community 12 - "CLI Commands"
Cohesion: 0.15
Nodes (24): baseURL(), cmdAgent(), cmdConformance(), cmdDiagnose(), cmdDoctor(), cmdOps(), cmdSandbox(), cmdSource() (+16 more)

### Community 13 - "Authz Service Tests"
Cohesion: 0.10
Nodes (26): testing.T, TestListEdgesOrdersBeforeApplyingLimit(), TestBatchCheckPutsConsistencyOnRequest(), app.ApplicationService, newP1Svc(), TestIntakeBatch(), TestOpsLagDoesNotClaimOutbox(), TestRegisterSourceRequireOrgDeny() (+18 more)

### Community 14 - "Tenant/Authz Test Suite"
Cohesion: 0.18
Nodes (24): NewEvidence(), NewIndex(), NewStore(), TestTenantIsolation(), NewMemory(), changeItemCount(), TestE2EMemoryHappyPathAndRevoke(), TestAuthzOutboxDrainAndReconcile() (+16 more)

### Community 15 - "Event Bus Audit Log"
Cohesion: 0.09
Nodes (16): noopBus, MemoryLogger, sync.Mutex, sanitize(), AuditEvent, EventMessage, InboxEntry, IntakeEvent (+8 more)

### Community 16 - "Service Dependency Schema"
Cohesion: 0.08
Nodes (31): required, required, enum, jetstream_persistence, openfga_store, pgvector, s3_object_lock, s3_versioning (+23 more)

### Community 17 - "Context Fabric API Tools"
Cohesion: 0.10
Nodes (29): ADR 0015: Sparse-first recall, AuthZ can_read BatchCheck, OpenFGA Authorization Model, context.brief tool, context.get tool, context.graph tool, context.request_access tool, context.search tool (+21 more)

### Community 18 - "Index Search Filters"
Cohesion: 0.12
Nodes (16): classRank(), Index, matchFilters(), attr(), classRank(), pgx.Tx, isMissingFTSColumn(), itoa() (+8 more)

### Community 19 - "Authz Worker Reconciliation"
Cohesion: 0.14
Nodes (9): jetStreamFetcher, noopSub, outboxPayload, Worker, sync.WaitGroup, RunWorker(), EnqueueForEdge(), NeedsSynchronization() (+1 more)

### Community 20 - "Test Case Reporting"
Cohesion: 0.13
Nodes (24): Case, caseFn, Report, Result, RunOptions, Suite, HasContentFields(), New() (+16 more)

### Community 21 - "Export Job Schema"
Cohesion: 0.08
Nodes (26): description, type, format, type, type, description, format, minLength (+18 more)

### Community 22 - "Resource Store Interfaces"
Cohesion: 0.11
Nodes (20): ChangeLister, ExportJobStore, ExtraStore, GraphRequest, IntakeRequest, MappingStore, memoryAuthzSeeder, QuotaStore (+12 more)

### Community 23 - "Mapping Spec Schema"
Cohesion: 0.08
Nodes (25): const, type, minLength, type, null, additionalProperties, type, minLength (+17 more)

### Community 24 - "Source Config Schema"
Cohesion: 0.08
Nodes (25): $ref, $ref, format, type, maxLength, type, minLength, type (+17 more)

### Community 25 - "Organization Store Records"
Cohesion: 0.11
Nodes (5): time.Time, ErrNotFound(), IdempotencyRecord, Organization, PresignOptions

### Community 26 - "Context Packet Schema"
Cohesion: 0.08
Nodes (24): type, type, description, type, type, type, properties, audit_id (+16 more)

### Community 27 - "Helm Chart Template"
Cohesion: 0.09
Nodes (23): type, $ref, type, $ref, type, type, type, properties (+15 more)

### Community 28 - "Ingest Bucket Schema"
Cohesion: 0.09
Nodes (23): properties, required, type, derived, raw, buckets, additionalProperties, properties (+15 more)

### Community 29 - "Service Config Schema"
Cohesion: 0.10
Nodes (23): properties, database, host, issuer, port, required, type, required (+15 more)

### Community 30 - "Backup Store Config"
Cohesion: 0.13
Nodes (23): type, type, $ref, type, $ref, pattern, type, properties (+15 more)

### Community 31 - "TypeScript Package Config"
Cohesion: 0.09
Nodes (22): openapi-typescript, dist, README.md, src, description, devDependencies, openapi-typescript, typescript (+14 more)

### Community 32 - "Main Bootstrap Config"
Cohesion: 0.23
Nodes (21): allowMemoryAuth(), allowStubOps(), asRelWriter(), firstEnv(), app.ApplicationService, locateOpsScript(), main(), profileName() (+13 more)

### Community 33 - "Webhook Signing Config"
Cohesion: 0.09
Nodes (22): type, enum, type, key_id, method, replay_window_seconds, secret_ref, signing (+14 more)

### Community 34 - "NATS Event Bus"
Cohesion: 0.14
Nodes (13): Connect(), ConnectOrNoop(), headerMap(), NewNoop(), EventBus, Bus, nats.Conn, nats.Header (+5 more)

### Community 35 - "Storage Config Schema"
Cohesion: 0.19
Nodes (21): required, buckets, database, endpoint, issuer, region, required, required (+13 more)

### Community 36 - "Change Event Schema"
Cohesion: 0.10
Nodes (20): type, format, type, minLength, type, properties, brand_id, context_space_id (+12 more)

### Community 37 - "Export Evidence Bundle"
Cohesion: 0.24
Nodes (14): AuthzTupleManifest, ContentEntry, EvidenceRef, Job, Tombstone, collectEvidenceRefs(), entry(), Manifest (+6 more)

### Community 38 - "API Key Auth Resolver"
Cohesion: 0.14
Nodes (12): AgentClaims, APIKeyResolver, Chained, NewAPIKeyResolver(), looksLikeJWT(), CredentialProvider, IdentityProvider, IndexProvider (+4 more)

### Community 39 - "Source Registration Schema"
Cohesion: 0.11
Nodes (19): id, source_id, status, required, organization_id, retention_policy_id, status, required (+11 more)

### Community 40 - "Agent Credential Store"
Cohesion: 0.19
Nodes (9): randomSecret(), NewCredentialStore(), randomSecret(), AgentCredential, AgentPrincipal, CreateAgentCredentialRequest, agentCred, CredentialStore (+1 more)

### Community 41 - "Audit Ledger Signing"
Cohesion: 0.18
Nodes (12): LedgerLogger, LedgerWriter, ChangeAppender, CompletionManifest, DerivativeRef, LegalHoldChecker, Request, Logger (+4 more)

### Community 42 - "App Config Loader"
Cohesion: 0.25
Nodes (16): Config, ConnectionMode, MCPConfig, NATSConfig, OIDCConfig, OpenFGAConfig, PostgresConfig, S3Config (+8 more)

### Community 43 - "S3 Evidence Store"
Cohesion: 0.15
Nodes (11): harness, github.com/aws/aws-sdk-go-v2/service/s3.Client, github.com/aws/aws-sdk-go-v2/service/s3.PresignClient, io.ReadCloser, io.Reader, sync.RWMutex, EvidenceStore, HashBody() (+3 more)

### Community 44 - "Change Event Metadata"
Cohesion: 0.12
Nodes (18): occurred_at, schema_version, required, additionalProperties, required, type, trust, data (+10 more)

### Community 45 - "Resource Provenance Schema"
Cohesion: 0.13
Nodes (18): required, required, context_space_id, purpose_allowlist, resource_id, resource_type, revision_id, trust (+10 more)

### Community 46 - "Graph Edge Schema"
Cohesion: 0.11
Nodes (18): maximum, minimum, type, type, type, properties, minLength, type (+10 more)

### Community 47 - "Graph Edge Sync Schema"
Cohesion: 0.11
Nodes (18): $ref, default, description, type, description, $ref, properties, description (+10 more)

### Community 48 - "Rate Quota Schema"
Cohesion: 0.11
Nodes (18): minimum, type, minimum, type, default, maximum, minimum, type (+10 more)

### Community 49 - "Backup Policy Schema"
Cohesion: 0.11
Nodes (18): additionalProperties, properties, required, type, items, minItems, type, uniqueItems (+10 more)

### Community 50 - "Dependency Config Schema"
Cohesion: 0.11
Nodes (18): type, $defs, connectionMode, natsDependency, oidcDependency, openfgaDependency, postgresDependency, s3Dependency (+10 more)

### Community 51 - "Network Serve Config"
Cohesion: 0.11
Nodes (18): additionalProperties, properties, required, type, const, type, network, private_dependencies (+10 more)

### Community 52 - "Mapping Spec Status"
Cohesion: 0.15
Nodes (17): type, null, type, type, format, type, null, expires_at (+9 more)

### Community 53 - "Record Lifecycle States"
Cohesion: 0.15
Nodes (17): INDEXED, enum, type, lifecycle_state, ACCEPTED, INDEXED, PROJECTING, PURGE_PENDING (+9 more)

### Community 54 - "Graph Node Schema"
Cohesion: 0.12
Nodes (17): type, additionalProperties, type, type, GraphNode, additionalProperties, properties, required (+9 more)

### Community 55 - "Deployment Profile Config"
Cohesion: 0.12
Nodes (17): required, worker, required, config, dependencies, kind, profile, required (+9 more)

### Community 56 - "Record Classification Schema"
Cohesion: 0.12
Nodes (17): $ref, properties, type, type, classification, mapping_spec_id, mapping_spec_revision, retention_policy_id (+9 more)

### Community 57 - "Brief Request Schema"
Cohesion: 0.12
Nodes (17): format, type, default, type, default, description, type, default (+9 more)

### Community 58 - "Evidence Provenance Schema"
Cohesion: 0.12
Nodes (17): $ref, description, type, properties, classification, content_locator, source_authority, source_event_id (+9 more)

### Community 59 - "Resource Type Enum"
Cohesion: 0.13
Nodes (17): artifact, document, event, fact, message, observation, resource_type, enum (+9 more)

### Community 60 - "OIDC Client Config"
Cohesion: 0.12
Nodes (17): minLength, type, type, type, format, type, format, type (+9 more)

### Community 61 - "Intake Ingest Service"
Cohesion: 0.20
Nodes (14): Result, CheckReplayWindow(), defaultSpec(), deterministicEdgeID(), firstNonEmpty(), IntakeService, Request, isSourceOfTruth() (+6 more)

### Community 62 - "OIDC Client Config Alt"
Cohesion: 0.12
Nodes (16): minLength, type, minLength, type, type, type, minLength, type (+8 more)

### Community 63 - "Schema Migration Config"
Cohesion: 0.12
Nodes (16): const, description, type, description, enum, type, additionalProperties, properties (+8 more)

### Community 64 - "TLS Connection Config"
Cohesion: 0.12
Nodes (16): description, type, type, tlsConfig, type, ca_secret_ref, client_cert_secret_ref, enabled (+8 more)

### Community 65 - "Storage Capability Schema"
Cohesion: 0.12
Nodes (16): properties, description, type, description, type, description, type, jetstream_persistence (+8 more)

### Community 66 - "OIDC Claim Mapping"
Cohesion: 0.12
Nodes (16): additionalProperties, properties, type, default, type, default, type, default (+8 more)

### Community 67 - "Local Auth Tests"
Cohesion: 0.15
Nodes (13): copyCapped(), TestCopyCappedAcceptsExactMax(), TestCopyCappedRejectsOversized(), TestLocalAgentRole(), TestLocalAuthenticate(), TestLocalDiscover(), TestLocalRejectsMalformed(), toolErrorResult() (+5 more)

### Community 68 - "Chatwoot Connector"
Cohesion: 0.27
Nodes (13): CloudEvent, Config, Connector, decodeLooseYAML(), dig(), firstEnv(), MappingSpecJSON(), New() (+5 more)

### Community 69 - "Domain Resource Enum"
Cohesion: 0.13
Nodes (15): connector role, enum, ADR 0004: Generic Intake and MappingSpec, ADR 0005: Generated Observations vs Agent Actions, audit, authz_model_pin, authz_tuples, delegation_grants (+7 more)

### Community 70 - "Delegation Grant Schema"
Cohesion: 0.13
Nodes (15): pattern, type, $ref, minLength, type, minLength, type, properties (+7 more)

### Community 71 - "Content Blob Schema"
Cohesion: 0.13
Nodes (15): minimum, type, properties, type, minLength, type, byte_size, kind (+7 more)

### Community 72 - "Context Space Schema"
Cohesion: 0.13
Nodes (15): $ref, $ref, $ref, properties, brand_id, content_locator, context_space_id, purpose_allowlist (+7 more)

### Community 73 - "Content Object Schema"
Cohesion: 0.13
Nodes (15): minimum, type, type, properties, minLength, type, byte_size, content_type (+7 more)

### Community 74 - "S3 Storage Config"
Cohesion: 0.13
Nodes (15): type, minLength, type, type, accessKeyKey, endpoint, pathStyle, region (+7 more)

### Community 75 - "Database Credentials Config"
Cohesion: 0.13
Nodes (15): type, minLength, type, type, type, minLength, type, properties (+7 more)

### Community 76 - "Rate Limiter"
Cohesion: 0.24
Nodes (10): DefaultLimits(), Limiter, NewLimiter(), TestLimiterOperations(), TestLimiterRateLimits(), TestLimiterScopesSeparately(), bucket, Key (+2 more)

### Community 77 - "Content Bundle Schema"
Cohesion: 0.14
Nodes (14): items, minItems, type, items, type, uniqueItems, additionalProperties, required (+6 more)

### Community 78 - "Field Transform Schema"
Cohesion: 0.14
Nodes (14): description, minLength, type, properties, default, expr, transform, enum (+6 more)

### Community 79 - "Plugin Manifest Schema"
Cohesion: 0.14
Nodes (13): api_version, additionalProperties, description, $id, id, kind, runtime, required (+5 more)

### Community 80 - "Plugin Version Schema"
Cohesion: 0.14
Nodes (14): const, type, format, type, type, pattern, type, properties (+6 more)

### Community 81 - "K8s Service Account"
Cohesion: 0.14
Nodes (14): type, additionalProperties, type, type, type, annotations, create, name (+6 more)

### Community 82 - "Storage Feature Flags"
Cohesion: 0.14
Nodes (14): additionalProperties, properties, type, type, type, type, capabilities, jetstream_persistence (+6 more)

### Community 83 - "Server Runtime Config"
Cohesion: 0.14
Nodes (14): properties, minLength, type, enum, type, minLength, type, listenAddr (+6 more)

### Community 84 - "Health Probe Config"
Cohesion: 0.14
Nodes (14): probe, minimum, type, path, minLength, type, minimum, type (+6 more)

### Community 85 - "Pod Scaling Config"
Cohesion: 0.15
Nodes (14): type, minimum, type, properties, type, enabled, minAvailable, podDisruptionBudget (+6 more)

### Community 86 - "Demo Deployment Profile"
Cohesion: 0.14
Nodes (14): demo, starter, xsama, enum, type, profile, demo, scaled (+6 more)

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

### Community 93 - "Classification Taxonomy"
Cohesion: 0.15
Nodes (12): classification, additionalProperties, description, $id, purpose, trust, required, $schema (+4 more)

### Community 94 - "K8s Service Ports"
Cohesion: 0.15
Nodes (13): maximum, minimum, type, port, service, type, properties, type (+5 more)

### Community 95 - "Public Surface Roles"
Cohesion: 0.15
Nodes (13): type, public_surface, roles, description, items, minItems, type, items (+5 more)

### Community 96 - "API Error Types"
Cohesion: 0.42
Nodes (9): baseErr(), ErrConflict(), ErrForbidden(), ErrRateLimited(), ErrUnauthorized(), ErrUnavailable(), TestAPIErrorHelpers(), TestErrRateLimitedRetryAfter() (+1 more)

### Community 97 - "TypeScript Compiler Config"
Cohesion: 0.15
Nodes (12): src/**/*.ts, compilerOptions, declaration, esModuleInterop, module, moduleResolution, outDir, rootDir (+4 more)

### Community 98 - "Context Action Enum"
Cohesion: 0.17
Nodes (12): description, items, minItems, type, uniqueItems, enum, type, allowed_actions (+4 more)

### Community 99 - "Access Grant Schema"
Cohesion: 0.17
Nodes (12): owner, status, required, actor, agent_principal, allowed_actions, classification_ceiling, created_by (+4 more)

### Community 100 - "Evidence Object Schema"
Cohesion: 0.17
Nodes (12): additionalProperties, properties, type, type, evidence, object_version, sha256, uri (+4 more)

### Community 101 - "Archive Manifest Checksums"
Cohesion: 0.17
Nodes (12): pattern, type, additionalProperties, properties, required, type, pattern, type (+4 more)

### Community 102 - "Case Classification Tags"
Cohesion: 0.17
Nodes (12): description, $ref, $ref, $ref, properties, $ref, brand_tag, case_type (+4 more)

### Community 103 - "Egress and Secrets Config"
Cohesion: 0.18
Nodes (12): default, items, type, uniqueItems, minLength, type, egress_allowlist, secrets (+4 more)

### Community 104 - "Credential Config Schema"
Cohesion: 0.17
Nodes (12): properties, type, type, minLength, type, properties, bundled, credentialsKey (+4 more)

### Community 105 - "Deployment Resource Config"
Cohesion: 0.17
Nodes (12): minLength, type, properties, domain, replicas, storage, minimum, type (+4 more)

### Community 106 - "Endpoint Schema"
Cohesion: 0.17
Nodes (12): properties, minLength, type, type, maximum, minimum, type, host (+4 more)

### Community 107 - "Ingest Idempotency Tests"
Cohesion: 0.42
Nodes (11): baseSpec(), boolPtr(), setup(), signedReq(), TestDuplicateIntakeIdempotent(), TestGeneratedObservationCannotOverwrite(), TestHMACFail(), TestIntakeEmitsParentEdgeFromVisibilityRef() (+3 more)

### Community 108 - "Context Packet Schema"
Cohesion: 0.18
Nodes (10): additionalProperties, additionalProperties, type, $defs, Citation, description, $id, $schema (+2 more)

### Community 109 - "Citation Schema"
Cohesion: 0.18
Nodes (11): type, properties, citation_id, resource_id, revision_id, score, snippet, type (+3 more)

### Community 110 - "Verification Status Enum"
Cohesion: 0.20
Nodes (11): revoked, status, enum, type, active, status, enum, type (+3 more)

### Community 111 - "Event Metadata Fields"
Cohesion: 0.18
Nodes (11): type, type, properties, causationid, correlationid, subject, traceparent, description (+3 more)

### Community 112 - "Job Status Enum"
Cohesion: 0.18
Nodes (11): status, enum, type, FAILED, never, enum, cancelled, completed (+3 more)

### Community 113 - "Controlled Tag Prefixes"
Cohesion: 0.18
Nodes (11): description, items, type, uniqueItems, enum, type, controlled_tag_prefixes, brand: (+3 more)

### Community 114 - "API Credential Config"
Cohesion: 0.18
Nodes (11): type, minLength, type, minLength, type, properties, apiTokenKey, apiUrl (+3 more)

### Community 115 - "Service Network Surface"
Cohesion: 0.18
Nodes (11): enum, type, serve, properties, type, type, network, privateDependencies (+3 more)

### Community 116 - "Required Dependencies Enum"
Cohesion: 0.22
Nodes (11): required, enum, nats, oidc, postgres, s3, enum, enum (+3 more)

### Community 117 - "Database Migration Runner"
Cohesion: 0.38
Nodes (9): runMigrate(), AdminDSN(), CheckApplied(), loadEmbedded(), loadMigrations(), MigrationsDir(), Run(), migrationFile (+1 more)

### Community 118 - "Action Restrictions Schema"
Cohesion: 0.20
Nodes (10): items, type, items, type, type, items, type, action_restrictions (+2 more)

### Community 119 - "Usage Budget Limits"
Cohesion: 0.20
Nodes (10): additionalProperties, properties, type, minimum, type, minimum, type, budget (+2 more)

### Community 120 - "Attribute Type Schema"
Cohesion: 0.22
Nodes (10): type, additionalProperties, description, type, null, attributes, type, boolean (+2 more)

### Community 121 - "tombstone"
Cohesion: 0.20
Nodes (10): description, type, null, object, brand_id, supersedes_revision_id, tombstone, type (+2 more)

### Community 122 - "Allowed Record Types"
Cohesion: 0.20
Nodes (10): items, minItems, type, uniqueItems, items, type, uniqueItems, type (+2 more)

### Community 123 - "Job Workload Spec"
Cohesion: 0.22
Nodes (10): $defs, jobSpec, roleWorkload, required, type, required, type, enabled (+2 more)

### Community 124 - "Database Secret Refs"
Cohesion: 0.20
Nodes (10): description, type, minLength, type, properties, admin_secret_ref, database, role_secret_ref (+2 more)

### Community 125 - "Name Version Schema"
Cohesion: 0.20
Nodes (10): type, properties, maxLength, minLength, type, description, name, version (+2 more)

### Community 126 - "Jetstream Storage Config"
Cohesion: 0.20
Nodes (10): additionalProperties, description, enum, required, type, jetstream, rebuild_from_outbox, replicas (+2 more)

### Community 127 - "Change Event Schema"
Cohesion: 0.22
Nodes (8): additionalProperties, description, $id, not, anyOf, $schema, title, type

### Community 128 - "Resource Change Events"
Cohesion: 0.22
Nodes (9): enum, type, change_type, resource.accepted, resource.failed, resource.indexed, resource.purged, resource.reclassified (+1 more)

### Community 129 - "Purpose Classification Enum"
Cohesion: 0.33
Nodes (9): purpose, enum, type, account_management, agent_assist, finance, marketing, support (+1 more)

### Community 130 - "Dominance Decision Schema"
Cohesion: 0.22
Nodes (9): const, type, dominates, reason, requested_at, type, format, type (+1 more)

### Community 131 - "Visibility Mapping Policy"
Cohesion: 0.22
Nodes (9): type, enum, type, default_visibility_ref, mode, properties, deny_by_default, fixed_ref (+1 more)

### Community 132 - "Mapping Spec Schema"
Cohesion: 0.22
Nodes (9): properties, type, type, mapping_spec_id, mapping_spec_revision, schema_id, version, type (+1 more)

### Community 133 - "Health Probes Schema"
Cohesion: 0.22
Nodes (9): $ref, properties, type, liveness, probes, readiness, startup, $ref (+1 more)

### Community 134 - "Service Dependency Refs"
Cohesion: 0.22
Nodes (9): properties, $ref, $ref, $ref, nats, oidc, postgres, s3 (+1 more)

### Community 135 - "Auth Credential Tests"
Cohesion: 0.39
Nodes (7): NewCredentialStore(), TestRevokeIsOrgScoped(), TestChainedAPIKeyBearerAuthenticates(), TestChainedAPIKeyHeaderField(), TestChainedLocalTokenStillWorks(), TestChainedRejectsUnknownKey(), WithAPIKeys()

### Community 137 - "Synthetic Connector Runner"
Cohesion: 0.46
Nodes (6): runConnector(), env(), New(), RunCLI(), Config, Connector

### Community 138 - "Authorization Fixture Tests"
Cohesion: 0.46
Nodes (7): authorizationFixture, evaluateFixture(), fixturesDir(), principalKind(), seedSampleTuples(), stripPrefix(), TestAuthorizationFixturesAllowDenyMatrix()

### Community 139 - "Redaction Schema"
Cohesion: 0.25
Nodes (8): Redaction, type, profile, reason_code, type, additionalProperties, properties, type

### Community 140 - "Error Response Schema"
Cohesion: 0.25
Nodes (8): additionalProperties, properties, type, type, error, message, reason_code, type

### Community 141 - "Authority Constraints Schema"
Cohesion: 0.25
Nodes (8): additionalProperties, default, type, authority_ceiling_from_source, cannot_broaden_acl, cannot_mint_organization, client_fields_may_only_narrow, constraints

### Community 142 - "Timestamp Fields Schema"
Cohesion: 0.25
Nodes (8): $ref, $ref, observed_at, occurred_at, timestamps, additionalProperties, properties, type

### Community 143 - "Forbidden Direct Access"
Cohesion: 0.25
Nodes (8): broad_evidence_credentials, direct_database, direct_nats, direct_openfga, additionalProperties, default, type, forbidden

### Community 144 - "Emitted Event Types"
Cohesion: 0.25
Nodes (8): items, minItems, type, uniqueItems, enum, emits, context.ingest.request.v1, context.lifecycle.notification.v1

### Community 145 - "Tags Schema"
Cohesion: 0.25
Nodes (8): maxLength, minLength, type, tags, description, items, type, uniqueItems

### Community 146 - "Verification Status Schema"
Cohesion: 0.25
Nodes (8): type, format, last_status, last_verified_at, verification, additionalProperties, properties, type

### Community 147 - "Classification Levels Enum"
Cohesion: 0.25
Nodes (8): description, enum, type, classification, confidential, internal, public, restricted

### Community 148 - "Service Config Schema"
Cohesion: 0.25
Nodes (8): additionalProperties, required, type, config, capabilities, listenAddr, logLevel, openfgaModelId

### Community 149 - "Ownership Enum"
Cohesion: 0.32
Nodes (8): enum, description, enum, type, owner, operator, platform, shared

### Community 150 - "Prediction Scoring Tool"
Cohesion: 0.46
Nodes (7): loadPreds(), loadTasks(), main(), percentile(), setOf(), prediction, task

### Community 152 - "OpenAPI Generation Script"
Cohesion: 0.25
Nodes (7): bin, here, openapi, outDir, outFile, r, root

### Community 153 - "Graph Edge Schema"
Cohesion: 0.29
Nodes (7): GraphEdge, additionalProperties, required, type, edge_id, from_id, to_id

### Community 154 - "Audit Authorization Record"
Cohesion: 0.29
Nodes (7): required, action_restrictions, audit_id, authz_revision, citations, policy_revision, redactions

### Community 155 - "Delegation Grant Schema"
Cohesion: 0.29
Nodes (6): additionalProperties, description, $id, $schema, title, type

### Community 156 - "Event Envelope Schema"
Cohesion: 0.29
Nodes (6): additionalProperties, description, $id, $schema, title, type

### Community 157 - "CloudEvents Envelope Schema"
Cohesion: 0.29
Nodes (7): id, required, data, datacontenttype, specversion, time, type

### Community 158 - "Schema Version List"
Cohesion: 0.29
Nodes (7): schema_id, additionalProperties, required, schema_versions, items, minItems, type

### Community 159 - "Export Manifest Schema"
Cohesion: 0.29
Nodes (6): additionalProperties, description, $id, $schema, title, type

### Community 160 - "Export Manifest Status"
Cohesion: 0.29
Nodes (7): status, required, checksums, contents, created_at, export_id, format_version

### Community 161 - "Capabilities Schema"
Cohesion: 0.29
Nodes (7): description, items, minItems, type, uniqueItems, pattern, capabilities

### Community 162 - "Pipeline Stage Kind"
Cohesion: 0.29
Nodes (7): enum, type, kind, source:, enricher, parser, transform

### Community 163 - "Record Schema"
Cohesion: 0.29
Nodes (6): additionalProperties, description, $id, $schema, title, type

### Community 164 - "Evidence Object Schema"
Cohesion: 0.29
Nodes (7): additionalProperties, required, type, sha256, evidence, object_version, uri

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

### Community 169 - "Job Backoff Config"
Cohesion: 0.29
Nodes (7): minimum, type, properties, backoffLimit, ttlSecondsAfterFinished, minimum, type

### Community 170 - "Config Schema"
Cohesion: 0.29
Nodes (6): additionalProperties, description, $id, $schema, title, type

### Community 171 - "Evidence Ledger Schema"
Cohesion: 0.29
Nodes (7): description, type, description, type, properties, evidence, ledger

### Community 172 - "Persistence Ledger Config"
Cohesion: 0.29
Nodes (7): additionalProperties, required, type, persistence, evidence, jetstream, ledger

### Community 173 - "ID Generation Utility"
Cohesion: 0.62
Nodes (6): TestNewIDs(), NewArtifactID(), NewEventID(), NewResourceID(), NewRevisionID(), newTimedID()

### Community 174 - "OpenFGA Bootstrap"
Cohesion: 0.53
Nodes (6): bootstrapOpenFGA(), firstNonEmpty(), openfgaCreateStore(), openfgaPingStore(), openfgaWriteAuthorizationModel(), net/http.Client

### Community 175 - "Resource Selector Schema"
Cohesion: 0.33
Nodes (6): pattern, resource_selector, items, minItems, type, uniqueItems

### Community 176 - "Purpose Allowlist Schema"
Cohesion: 0.33
Nodes (6): $ref, purpose_allowlist, items, minItems, type, uniqueItems

### Community 177 - "Send Policy Enum"
Cohesion: 0.33
Nodes (6): send_policy, default, enum, type, denied, human_approval_required

### Community 178 - "Document Status Enum"
Cohesion: 0.33
Nodes (6): status, enum, type, deprecated, draft, reviewed

### Community 179 - "Trust Tier Enum"
Cohesion: 0.33
Nodes (6): trust_tier, enum, type, deterministic_transform, first_party_connector, third_party_plugin

### Community 180 - "Purpose Allowlist Schema"
Cohesion: 0.33
Nodes (6): $ref, purpose_allowlist, items, minItems, type, uniqueItems

### Community 181 - "Visibility Mapping Policy"
Cohesion: 0.33
Nodes (6): visibility_mapping_policy, additionalProperties, required, type, allowed_visibility_refs, mode

### Community 182 - "Domain Enum"
Cohesion: 0.33
Nodes (6): enum, type, domain, engineering, hr, sales

### Community 183 - "Values Schema"
Cohesion: 0.33
Nodes (5): additionalProperties, $id, $schema, title, type

### Community 184 - "Connection Mode Enum"
Cohesion: 0.40
Nodes (6): enum, type, connectionMode, enum, bundled, external

### Community 185 - "Endpoint Schema"
Cohesion: 0.33
Nodes (6): endpoint, additionalProperties, required, type, host, port

### Community 186 - "Failure Behavior Enum"
Cohesion: 0.33
Nodes (6): default, enum, type, failure_behavior, degraded_read_only, fail_closed

### Community 187 - "Metadata Schema"
Cohesion: 0.33
Nodes (6): version, additionalProperties, required, type, metadata, name

### Community 188 - "OpenFGA Store Schema"
Cohesion: 0.33
Nodes (6): description, enum, $ref, type, openfga, store_and_tuples

### Community 190 - "Purpose Allowlist Schema"
Cohesion: 0.40
Nodes (5): $ref, purpose_allowlist, items, type, uniqueItems

### Community 191 - "Tags List Schema"
Cohesion: 0.40
Nodes (5): type, tags, items, type, uniqueItems

### Community 192 - "Example ID Pattern"
Cohesion: 0.40
Nodes (5): examples, pattern, type, id, com.example.chatwoot-source

### Community 193 - "SBOM Signature Refs"
Cohesion: 0.40
Nodes (5): null, sbom_attestation_ref, signature_ref, type, type

### Community 194 - "Runtime Enum"
Cohesion: 0.40
Nodes (5): runtime, enum, type, oci, wasm

### Community 195 - "Evidence Provenance Enum"
Cohesion: 0.40
Nodes (5): enum, corroborating, inferred, source_of_truth, user_claim

### Community 196 - "Authority Purpose Schema"
Cohesion: 0.40
Nodes (5): type, properties, authority, purpose, type

### Community 197 - "Required Services List"
Cohesion: 0.40
Nodes (5): required, nats, oidc, s3, openfga

### Community 198 - "Policy Evaluation Logic"
Cohesion: 0.50
Nodes (3): rank(), PolicyResult, Provider

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

### Community 203 - "Revoked At Schema Field"
Cohesion: 0.50
Nodes (4): null, revoked_at, format, type

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

### Community 209 - "Organization ID Schema Field"
Cohesion: 0.50
Nodes (4): description, minLength, type, organizationid

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

### Community 215 - "Valid Time Schema Field"
Cohesion: 0.50
Nodes (4): valid_time, description, format, type

### Community 216 - "Visibility Ref Schema Field"
Cohesion: 0.50
Nodes (4): visibility_ref, description, minLength, type

### Community 217 - "Display Name Schema Field"
Cohesion: 0.50
Nodes (4): maxLength, minLength, type, display_name

### Community 218 - "Dependencies Schema Field"
Cohesion: 0.50
Nodes (4): additionalProperties, type, type, dependencies

### Community 219 - "Dependencies Schema Field"
Cohesion: 0.50
Nodes (4): additionalProperties, type, type, dependencies

### Community 220 - "Model ID Schema Field"
Cohesion: 0.50
Nodes (4): description, minLength, type, model_id

### Community 221 - "S3 Store Config Code"
Cohesion: 0.83
Nodes (3): MaxBytesFromEnv(), New(), Config

### Community 222 - "Created At Schema Field"
Cohesion: 0.67
Nodes (3): format, type, created_at

### Community 223 - "Created By Schema Field"
Cohesion: 0.67
Nodes (3): pattern, type, created_by

### Community 224 - "Expires At Schema Field"
Cohesion: 0.67
Nodes (3): format, type, expires_at

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

### Community 235 - "Schema Version Schema Field"
Cohesion: 0.67
Nodes (3): schema_version, minLength, type

### Community 236 - "Spec Version Schema Field"
Cohesion: 0.67
Nodes (3): specversion, const, type

### Community 237 - "Time Schema Field"
Cohesion: 0.67
Nodes (3): time, format, type

### Community 238 - "Type Pattern Schema Field"
Cohesion: 0.67
Nodes (3): type, pattern, type

### Community 239 - "Controlled Taxonomy Schema Field"
Cohesion: 0.67
Nodes (3): additionalProperties, type, controlled_taxonomy

### Community 240 - "Description Schema Field"
Cohesion: 0.67
Nodes (3): maxLength, type, description

### Community 241 - "Parent Resource ID Schema Field"
Cohesion: 0.67
Nodes (3): description, $ref, parent_resource_id

### Community 242 - "Visibility Ref Schema Field"
Cohesion: 0.67
Nodes (3): visibility_ref, description, $ref

### Community 243 - "Content Hash Schema Field"
Cohesion: 0.67
Nodes (3): pattern, type, content_hash

### Community 244 - "Context Space ID Schema Field"
Cohesion: 0.67
Nodes (3): minLength, type, context_space_id

### Community 245 - "Retention Policy ID Schema Field"
Cohesion: 0.67
Nodes (3): retention_policy_id, minLength, type

### Community 246 - "Source External ID Schema Field"
Cohesion: 0.67
Nodes (3): source_external_id, minLength, type

### Community 247 - "Source Revision Schema Field"
Cohesion: 0.67
Nodes (3): source_revision, minLength, type

### Community 248 - "Source System Schema Field"
Cohesion: 0.67
Nodes (3): source_system, minLength, type

### Community 249 - "Title Schema Field"
Cohesion: 0.67
Nodes (3): title, maxLength, type

### Community 250 - "Path Style Schema Field"
Cohesion: 0.67
Nodes (3): default, type, path_style

### Community 251 - "Region Schema Field"
Cohesion: 0.67
Nodes (3): region, minLength, type

## Ambiguous Edges - Review These
- `Context Fabric Historical Reference Architecture` → `ADR 0012: V1 Boundaries`  [AMBIGUOUS]
  context-fabric-stress-test.md · relation: references

## Knowledge Gaps
- **1246 isolated node(s):** `AccessRequestBody`, `BriefRequest`, `Citation`, `ClientOptions`, `GraphEdge` (+1241 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 1293 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **15 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **What is the exact relationship between `Context Fabric Historical Reference Architecture` and `ADR 0012: V1 Boundaries`?**
  _Edge tagged AMBIGUOUS (relation: references) - confidence is low._
- **Why does `organization_id` connect `Source Registration Schema` to `Export Manifest Status`, `Audit Authorization Record`, `Change Event Metadata`, `Resource Provenance Schema`?**
  _High betweenness centrality (0.083) - this node is a cross-community bridge._
- **Why does `required` connect `Plugin Manifest Schema` to `Service Config Schema`?**
  _High betweenness centrality (0.081) - this node is a cross-community bridge._
- **Why does `properties` connect `Plugin Capabilities Schema` to `Container Image Schema`, `Config Schema`, `Persistence Ledger Config`, `Metadata Schema`, `Backup Policy Schema`, `Network Serve Config`, `Demo Deployment Profile`, `Dependencies Schema Field`, `Public Surface Roles`, `Schema Migration Config`?**
  _High betweenness centrality (0.079) - this node is a cross-community bridge._
- **What connects `AccessRequestBody`, `BriefRequest`, `Citation` to the rest of the system?**
  _1246 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `HTTP JSON Handler` be split into smaller, more focused modules?**
  _Cohesion score 0.08015989901115085 - nodes in this community are weakly interconnected._
- **Should `Authz Ledger Store` be split into smaller, more focused modules?**
  _Cohesion score 0.0662004662004662 - nodes in this community are weakly interconnected._