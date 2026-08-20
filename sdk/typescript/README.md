# TypeScript client (stub)

This package is a placeholder for a generated OpenAPI client.

## Source of truth

Generate types and fetch wrappers from:

[`contracts/openapi/openapi.yaml`](../../contracts/openapi/openapi.yaml)

Suggested tooling: `openapi-typescript` + `openapi-fetch`, or `openapi-generator`.

## Minimal usage shape

```ts
const base = process.env.CONTEXT_FABRIC_URL!;
const token = process.env.CONTEXT_FABRIC_TOKEN!;

export type GraphNode = {
  resource_id: string;
  kind?: string;
  title?: string;
  classification?: string;
  state?: string;
  attributes?: Record<string, string>;
};

export type GraphEdge = {
  edge_id: string;
  from_id: string;
  to_id: string;
  predicate: string;
  confidence?: number;
  sync_authz?: boolean;
};

export type ContextPacket = {
  version: string;
  packet_id?: string;
  organization_id: string;
  purpose: string;
  nodes?: GraphNode[];
  edges?: GraphEdge[];
  citations: unknown[];
  redactions: unknown[];
  truncated?: boolean;
  next_cursor?: string;
};

export async function search(orgId: string, body: unknown): Promise<ContextPacket> {
  const res = await fetch(`${base}/v1/organizations/${orgId}/context:search`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function graph(
  orgId: string,
  body: {
    resource_id: string;
    purpose: string;
    depth?: number;
    max_nodes?: number;
    predicates?: string[];
  },
): Promise<ContextPacket> {
  const res = await fetch(`${base}/v1/organizations/${orgId}/context:graph`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}
```

Do not embed secrets in the SDK. Prefer audience-bound tokens and the MCP/OAuth protected-resource metadata at `/.well-known/oauth-protected-resource`.
