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

export async function search(orgId: string, body: unknown) {
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
```

Do not embed secrets in the SDK. Prefer audience-bound tokens and the MCP/OAuth protected-resource metadata at `/.well-known/oauth-protected-resource`.
