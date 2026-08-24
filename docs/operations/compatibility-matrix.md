# Compatibility matrix

Independent version lines for Context Fabric portable surfaces (ADR 0015).

| Component | Current | Notes |
|-----------|---------|-------|
| REST API | `1.0.0` (`contracts/openapi/openapi.yaml`) | Public HTTP contract |
| MCP protocol | `2024-11-05`, `2025-03-26` | Negotiated on `initialize` |
| ContextPacket | `context-fabric.packet/v1` | `contracts/jsonschema/context-packet.json` |
| Conformance suite | `contracts/conformance/suite.yaml` | In-process + `cf sandbox conformance` |
| Skills packs | `integrations/skills/` | **Non-authoritative**; never an AuthZ boundary |
| AuthZ model | `contracts/openfga/model.fga` (+ `model.json`) | Pin `OPENFGA_MODEL_ID` per deployment |

Bump rows independently; do not imply that a skills pack bump requires an API bump.
