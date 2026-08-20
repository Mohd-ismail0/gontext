# Self-hosted enterprise context layer: platform assessment

## Decision

**Build a thin, organization-owned context gateway on the existing PostgreSQL 18 + pgvector estate; do not designate an agent-memory/RAG product as the authorization system of record.** PostgreSQL RLS controls normal row reads/writes and defaults to deny when enabled without a policy.[22] That makes it the strongest fit for the requested customer/internal boundary. Keep source blobs in SeaweedFS, canonical document/chunk/ACL/provenance and embeddings in PostgreSQL, Valkey only for non-sensitive or authorization-keyed caching, and ClickHouse for immutable retrieval/audit events.

Use a mature *retrieval engine* only when measured workload outgrows pgvector:
- **Qdrant** is the cost-conscious first offload candidate: it supports payload-based tenant partitioning, dedicated shards, and tiered multitenancy; its docs advise a shared collection plus payload isolation for many small tenants.[2] It also has API-key/TLS/audit capabilities, but self-hosted instances are open by default and API keys scope at collection—not individual document—granularity.[31]
- **Weaviate** is the alternative when database-native tenant isolation/RBAC outweighs added operational cost. It stores each tenant on a separate shard and exposes permissions for users, roles, tenants, and data objects.[32][33] It is a separate vector platform, however, so it duplicates an already-available PostgreSQL capability.

**Select Haystack as the optional code-level orchestration/retrieval library**, not as the shared data plane. Its `DocumentStore` protocol supports document write/filter/delete operations, and its Agent supports tools, runtime state and MCP toolsets.[37][38] LlamaIndex is the close alternative if its connector/ingestion ecosystem is more valuable to the team; its ingestion pipeline supports transformations, remote-vector-store writes, caching, async execution, document deduplication/update handling and parallel processing.[35] Neither framework supplies enterprise authorization: the gateway must derive and inject the authorization predicate.

## Comparative shortlist

| Candidate | What it contributes | Tenant / ACL fit | API, ingestion, agent integration | Production verdict for this use case |
|---|---|---|---|---|
| **PostgreSQL + pgvector (existing)** | Canonical metadata, ACLs, provenance, embeddings and transactional writes in one place. pgvector adds vector similarity search to Postgres.[56] | **Best initial fit:** RLS can enforce per-principal row visibility/mutation; use `FORCE ROW LEVEL SECURITY`, non-owner application roles, and deny-by-default policies.[22] | Gateway supplies REST + MCP; write deterministic ingestion workers. | **Recommended foundation.** Lowest new operational cost and the only candidate here that naturally combines relational ACL joins, revocation and audit-ready transactions. |
| **Qdrant** | Dedicated vector search service; Apache-2.0.[51] | Tenant payload partitioning/sharding is strong, but payload filters are *query constraints*, not a replacement for document-level authorization. Collection-scoped keys do not express per-document/group ACLs.[2][31] | REST/gRPC/client ecosystem; pair with gateway-generated filters and an external ingestion worker. | **Recommended scale-out vector tier**, only after retrieval load/latency tests justify another stateful service. Never expose it directly to tenants/agents. |
| **Weaviate** | Cloud-native vector/object DB; BSD-3-Clause.[52] | Native per-tenant shards plus RBAC permissions over tenant/data resources.[32][33] Its structured filters support logical composition.[34] | REST/GraphQL/gRPC and batch imports; agent integration is application-side. | **Strong enterprise-vector alternative**, particularly if native DB RBAC is a hard requirement; likely more operationally expensive/complex than the existing Postgres path. |
| **Haystack** | Apache-2.0 modular retrieval/agent framework.[53] | No organization-wide identity/ACL plane; custom `DocumentStore`/retriever must always apply the gateway’s constraints.[37] | Pipelines, components, tool-using agents, MCP tool/MCP toolset support.[38] | **Preferred framework layer** for controlled services and explicit pipelines—not a shared knowledge platform. |
| **LlamaIndex** | MIT document/agent framework.[54] | Metadata filters are passed to the vector store; authorization remains application-owned. | Transform/cached/async/parallel ingestion and vector-store integration; agents use memory and tools.[35][36] | **Good alternative/adjunct** for connectors and document processing; do not use its metadata filters as the security boundary. |
| **R2R** | Batteries-included, RESTful agentic RAG: multimodal ingestion, hybrid search, graphs, agentic retrieval, auth/collections.[45] | Collection/user access is promising, but validate exact enforcement and fit with Logto before trusting it for customer boundaries. | Out-of-box REST API and full Docker mode.[45] | **POC only, not recommended as core now.** The public repo’s latest main commit shown by GitHub is nine months old, a material maintenance risk for a security-facing foundation.[45] |
| **Mem0 OSS** | Apache-2.0 long-term agent memory service/library.[48] | Its scopes (`user_id`, `agent_id`, `run_id`) are useful logical partitions, not demonstrated organization/group/document ACL enforcement. | Self-host bundle includes REST, dashboard, auth enabled by default, JWT/per-user API keys and OpenAPI; CRUD/search by scope.[39][40] | **Optional per-agent/user memory adjunct.** Place behind the gateway; do not store shared authoritative customer knowledge or rely on scope IDs as RBAC. |
| **Graphiti OSS / Zep** | Apache-2.0 temporal context graphs with provenance, incremental updates and hybrid graph/vector/full-text retrieval.[50][43] | Open-source Graphiti is self-managed; Zep’s documented RBAC/ABAC, audit, retention and tenant isolation are features of the managed proprietary service, not Graphiti.[43] | Graphiti provides an **experimental** MCP server with `group_id` filtering, FalkorDB/Neo4j backends and async processing.[44] | **Optional bounded graph-memory component** for evolving relationship/history tasks. It adds a graph database and ACL work; front it with the same gateway and disable its opt-out telemetry for local-only policy.[44] Zep itself does not meet the local-data requirement. |
| **Letta** | Apache-2.0 stateful-agent runtime/memory model.[49] | Letta explicitly says a multi-tenant App Server should be isolated per tenant/machine and that controller authorization is required; it also says tool visibility is not a security boundary.[41] | SDK/API/app-server runtime with persistent agent state, external tools and MCP tools.[42][41] | **Use only for isolated stateful-agent workloads.** Not an organization-wide knowledge gateway. The public `letta-ai/letta` repository says its archived V1 server is unsupported for production, so verify the current supported deployment path before adoption.[49] |
| **Dify** | Collaborative app/workflow/RAG workspace with self-host/VPC deployment claims.[55] | Workspace is its tenant model, but its license forbids operating a multi-tenant Dify environment without written authorization.[46] | Easy app-facing KB ingestion and an external-knowledge contract: calls `/retrieval`, passes an API key, query, and metadata conditions.[47] | **Consumer/UI layer only**, if licensing fits. It is valuable as a client of the gateway, but its external KB API passes a configured bearer key—not authenticated end-user claims—so it cannot independently enforce end-user RBAC.[47] |

## Recommended design and enforcement model

1. **Identity and authorization:** The gateway validates Logto OIDC JWTs, maps subject, organization, roles and group memberships into a request-scoped principal, and writes a signed audit event. It never accepts `tenant_id`, `user_id`, ACL filters or `group_id` from an agent as authoritative.
2. **Canonical policy/data model:** Model `organization`, `principal`, `group`, `document`, `document_version`, `chunk`, `chunk_acl`, `source`, and `ingestion_job` in PostgreSQL. Keep origin, content hash, extraction/chunking/embedder version, classification, retention/deletion status and citations/provenance alongside each result.
3. **RLS-first retrieval:** Set the principal/group claims transaction-locally; query only RLS-visible chunks/documents. Use a non-owner database role so it cannot bypass policy. Apply lexical + vector retrieval *after* authorization, return source/version/citation metadata, and log policy/retrieval decisions. PostgreSQL documents that superusers, `BYPASSRLS` roles and normally table owners bypass RLS—do not use those identities for request handling.[22]
4. **Scale without changing the contract:** If pgvector is insufficient, dual-write embeddings to Qdrant. The gateway derives server-side filters such as `org_id`, `classification`, allowed group IDs and active version; it post-verifies each candidate against canonical PostgreSQL ACL/provenance before returning it. Qdrant is private-network-only, TLS/API key/audit logging enabled.[31]
5. **Interface surface:** Offer a versioned REST API and a single MCP server with tools such as `search_context`, `get_document_excerpt`, `cite_sources`, and tightly authorized `ingest_*`/`delete_*`. Each tool receives identity from transport authentication, not tool arguments. Expose a Dify-compatible `/retrieval` adapter only with a dedicated service identity and a fixed audience/scope; for true end-user Dify retrieval, add a trusted identity-propagation layer rather than using its static connector key.[47]
6. **Ingestion:** Workers obtain files from SeaweedFS/connectors, malware/format-validate, extract/OCR, classify, ACL-tag, chunk deterministically, embed locally or through an approved internal model endpoint, then commit version + ACL + vectors atomically. Re-ingestion changes versions; immediate revocation/deletion must remove/disable both canonical and offloaded vectors.

## What not to do

- Do not make a vector DB payload filter, Mem0 entity scope, Graphiti `group_id`, a Dify workspace, or an MCP tool allowlist the only authorization check. Each can help partition/query data, but none replaces gateway-controlled authorization at the canonical data layer.[2][40][41][44]
- Do not expose vector/graph databases or the MCP server publicly/directly to model clients. Self-hosted Qdrant is insecure until explicitly configured; Graphiti’s MCP server is marked experimental.[31][44]
- Do not select Zep Cloud for this stated requirement: its enterprise governance is managed/proprietary, whereas Graphiti OSS is self-managed.[43]
- Do not assume Dify is Apache-2.0 for a hosted multi-tenant product; its repository license imposes a commercial-license requirement for that use.[46]

## Practical phased recommendation

**Phase 1 — build:** PostgreSQL RLS + pgvector + SeaweedFS + Logto-backed REST/MCP gateway. This uses the existing data plane and makes authorization enforceable in the canonical store.[22] Use Haystack *or* LlamaIndex inside ingestion/retrieval workers.[35][38] Exploit existing Valkey and ClickHouse before adding specialist infrastructure.

**Phase 2 — validate:** Test authorization adversarially (cross-org, role downgrade, document ACL change, cache hit after revocation, duplicate/old vector result), then measure ingestion throughput, p95 query latency, recall and database impact under representative corpus/tenant distributions.

**Phase 3 — specialize only where proven:** Add Qdrant for high-volume semantic retrieval; add Graphiti for a small number of temporal relationship-memory domains; add Mem0 for per-agent preferences.[2][40][44] Keep all three behind the same contract and canonical ACL verifier. Consider Weaviate instead of Qdrant only if its native tenant/RBAC model materially reduces your operational authorization burden enough to justify replacing/duplicating pgvector.[32][33]

## Sources

[2] https://qdrant.tech/documentation/manage-data/multitenancy
[22] https://www.postgresql.org/docs/current/ddl-rowsecurity.html
[31] https://qdrant.tech/documentation/security
[32] https://docs.weaviate.io/weaviate/manage-collections/multi-tenancy
[33] https://docs.weaviate.io/weaviate/configuration/rbac/manage-roles
[34] https://docs.weaviate.io/weaviate/search/filters
[35] https://docs.llamaindex.ai/en/stable/module_guides/loading/ingestion_pipeline
[36] https://docs.llamaindex.ai/en/stable/module_guides/deploying/agents
[37] https://docs.haystack.deepset.ai/docs/document-store
[38] https://docs.haystack.deepset.ai/docs/agent
[39] https://docs.mem0.ai/open-source/setup
[40] https://docs.mem0.ai/open-source/features/rest-api
[41] https://docs.letta.com/platform/app-server/integration-patterns
[42] https://docs.letta.com/guides/agents/overview
[43] https://help.getzep.com/zep-vs-graphiti
[44] https://github.com/getzep/graphiti/blob/main/mcp_server/README.md
[45] https://github.com/SciPhi-AI/R2R
[46] https://github.com/langgenius/dify/blob/main/LICENSE
[47] https://docs.dify.ai/en/cloud/use-dify/knowledge/external-knowledge-api
[48] https://github.com/mem0ai/mem0/blob/main/LICENSE
[49] https://github.com/letta-ai/letta/blob/main/LICENSE
[50] https://github.com/getzep/graphiti/blob/main/LICENSE
[51] https://github.com/qdrant/qdrant/blob/master/LICENSE
[52] https://github.com/weaviate/weaviate/blob/main/LICENSE
[53] https://github.com/deepset-ai/haystack/blob/main/LICENSE
[54] https://github.com/run-llama/llama_index
[55] https://github.com/langgenius/dify
[56] https://github.com/pgvector/pgvector
