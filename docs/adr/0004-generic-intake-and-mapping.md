# ADR 0004: Generic Intake and MappingSpec

## Status

Accepted

## Context

Making Chatwoot a special path prevents other platforms from integrating. Cross-platform use requires a standards-based contribution surface.

## Decision

1. **Generic intake** is source-bound CloudEvents over HTTP (single and batch), plus presigned evidence upload.
2. Each **source registration** fixes organization, allowed record types, trust/authority ceilings, retention/classification defaults, visibility-mapping policy, signing method, rate quota, and schema versions. Client fields may only narrow these controls.
3. A versioned **`MappingSpec`** maps source IDs/revisions, context-space/resource identity, timestamps, content locators, controlled taxonomy, visibility references, and **knowledge-graph edges** into canonical records (ADR 0013).
4. Mapping changes are reviewed, fixture-tested, versioned, dry-runnable, and replayable.
5. Mappings **cannot** mint organizations, broaden ACLs, or declare higher source authority than the source registration allows. Edge emission writes knowledge facts; only `parent` may optionally sync an OpenFGA inheritance tuple (never a direct reader grant).
6. Chatwoot is a connector that uses only these public APIs—no DB/NATS/OpenFGA credentials.

## Consequences

- Any SaaS, internal service, or agent platform can contribute context.
- A second synthetic connector must pass the same conformance suite without core changes.
- Intake grows the org knowledge graph automatically via `edges[]`, `parent_resource_id`, or auto-parent from `visibility_ref`.