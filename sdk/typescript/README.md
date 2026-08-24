# TypeScript client

Hand-written client aligned with [`contracts/openapi/openapi.yaml`](../../contracts/openapi/openapi.yaml).
Optional OpenAPI types: `npm run generate` (writes `src/generated/openapi.ts`).

## Install

```bash
cd sdk/typescript
npm install
npm run build
```

## Usage

```ts
import { createClient } from "@xsama/context-fabric";

const client = createClient({
  baseUrl: process.env.CONTEXT_FABRIC_URL!,
  token: process.env.CONTEXT_FABRIC_TOKEN!,
});

const packet = await client.search("org_acme_0001", {
  query: "refund policy",
  purpose: "support",
  max_items: 10,
});

const graph = await client.graph("org_acme_0001", {
  resource_id: "res_case_1",
  purpose: "support",
  depth: 2,
});
```

Do not embed secrets in the SDK. Prefer audience-bound tokens and the MCP/OAuth
protected-resource metadata at `/.well-known/oauth-protected-resource`.
