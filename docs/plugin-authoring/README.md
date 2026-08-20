# Plugin authoring

Connectors must use only public Context Fabric APIs:

1. Register a source (`POST .../context/sources`) with trust/authority ceilings and schema versions.
2. Bind a versioned `MappingSpec` (see `contracts/jsonschema/mapping-spec.json`).
3. Emit knowledge-graph edges via `mappings.parent_resource_id`, top-level `edges[]`, or `visibility_ref` (auto parent). Edges are facts; only `parent` may sync OpenFGA inheritance.
4. Upload evidence via presigned URLs when needed.
5. Submit signed CloudEvents to `POST .../context/intake` (or batch).

## Rules

- No PostgreSQL, NATS, OpenFGA, or broad evidence credentials.
- HMAC (or configured signature) required; replay windows enforced.
- MappingSpec cannot mint organizations, broaden ACLs, or raise trust/authority/classification ceilings.
- Pin image digests; provide SBOM and signature before production activation.
- Default-deny egress; allowlist only the source vendor and Context Fabric intake URL.

## Kits in-repo

- Chatwoot: `internal/connectors/chatwoot/` + `mapping.yaml`
- Synthetic conformance connector: `internal/connectors/synthetic/`
- Conformance suite: `contracts/conformance/suite.yaml`

## Certification checklist

- [ ] Manifest validates against `plugin-manifest.json`
- [ ] Golden fixture intake matches ledger/projection hash of generic HTTP intake
- [ ] Undeclared egress blocked
- [ ] Digest pinned and signature verified
- [ ] Source verify + rotate secrets exercised
