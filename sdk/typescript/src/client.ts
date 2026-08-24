/**
 * Hand-written TypeScript client aligned with contracts/openapi/openapi.yaml.
 * Regenerated OpenAPI types live in ./generated/openapi.ts (`npm run generate`).
 */

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

export type Citation = {
  resource_id: string;
  revision_id?: string;
  snippet?: string;
  [key: string]: unknown;
};

export type ContextPacket = {
  version: string;
  packet_id?: string;
  organization_id: string;
  purpose: string;
  nodes?: GraphNode[];
  edges?: GraphEdge[];
  citations: Citation[];
  redactions: unknown[];
  truncated?: boolean;
  next_cursor?: string;
  policy_revision?: string;
  authz_revision?: string;
  audit_id?: string;
  decision?: string;
  action_restrictions?: string[];
};

export type SearchRequest = {
  query: string;
  purpose: string;
  max_items?: number;
  cursor?: string;
  include_tags?: string[];
  resource_ids?: string[];
};

export type BriefRequest = {
  purpose: string;
  resource_ids?: string[];
  max_items?: number;
};

export type GraphRequest = {
  resource_id: string;
  purpose: string;
  depth?: number;
  max_nodes?: number;
  predicates?: string[];
};

export type AccessRequestBody = {
  purpose: string;
  resource_id: string;
  justification?: string;
};

export type IntakeRequest = {
  source_id: string;
  idempotency_key?: string;
  body: unknown;
  signature?: string;
  timestamp?: string;
};

export type ClientOptions = {
  baseUrl: string;
  /** Bearer token (e.g. audience-bound OAuth or local:<org>:<sub>:<role>). Never embed secrets in source. */
  token: string;
  fetch?: typeof fetch;
};

export class ContextFabricError extends Error {
  readonly status: number;
  readonly body: string;

  constructor(status: number, body: string) {
    super(`Context Fabric HTTP ${status}: ${body}`);
    this.name = "ContextFabricError";
    this.status = status;
    this.body = body;
  }
}

export class ContextFabricClient {
  readonly baseUrl: string;
  private readonly token: string;
  private readonly fetchImpl: typeof fetch;

  constructor(opts: ClientOptions) {
    this.baseUrl = opts.baseUrl.replace(/\/$/, "");
    this.token = opts.token;
    this.fetchImpl = opts.fetch ?? globalThis.fetch.bind(globalThis);
  }

  search(orgId: string, body: SearchRequest): Promise<ContextPacket> {
    return this.post(`/v1/organizations/${enc(orgId)}/context:search`, body);
  }

  get(
    orgId: string,
    resourceId: string,
    purpose: string,
  ): Promise<ContextPacket> {
    const q = new URLSearchParams({ purpose });
    return this.getJSON(
      `/v1/organizations/${enc(orgId)}/context/resources/${enc(resourceId)}?${q}`,
    );
  }

  brief(orgId: string, body: BriefRequest): Promise<ContextPacket> {
    return this.post(`/v1/organizations/${enc(orgId)}/context:brief`, body);
  }

  graph(orgId: string, body: GraphRequest): Promise<ContextPacket> {
    return this.post(`/v1/organizations/${enc(orgId)}/context:graph`, body);
  }

  requestAccess(orgId: string, body: AccessRequestBody): Promise<unknown> {
    return this.post(
      `/v1/organizations/${enc(orgId)}/context/access-requests`,
      body,
    );
  }

  intake(orgId: string, req: IntakeRequest): Promise<unknown> {
    const headers: Record<string, string> = {};
    if (req.signature) headers["X-Context-Fabric-Signature"] = req.signature;
    if (req.timestamp) headers["X-Context-Fabric-Timestamp"] = req.timestamp;
    if (req.idempotency_key) {
      headers["Idempotency-Key"] = req.idempotency_key;
    }
    return this.post(
      `/v1/organizations/${enc(orgId)}/context/intake`,
      req.body,
      {
        headers,
        query: { source_id: req.source_id },
      },
    );
  }

  private async getJSON<T>(path: string): Promise<T> {
    const res = await this.fetchImpl(`${this.baseUrl}${path}`, {
      method: "GET",
      headers: this.authHeaders(),
    });
    return this.parse<T>(res);
  }

  private async post<T>(
    path: string,
    body: unknown,
    extra?: { headers?: Record<string, string>; query?: Record<string, string> },
  ): Promise<T> {
    let url = `${this.baseUrl}${path}`;
    if (extra?.query) {
      const q = new URLSearchParams(extra.query);
      url += `?${q}`;
    }
    const res = await this.fetchImpl(url, {
      method: "POST",
      headers: {
        ...this.authHeaders(),
        "Content-Type": "application/json",
        ...(extra?.headers ?? {}),
      },
      body: JSON.stringify(body),
    });
    return this.parse<T>(res);
  }

  private authHeaders(): Record<string, string> {
    return { Authorization: `Bearer ${this.token}` };
  }

  private async parse<T>(res: Response): Promise<T> {
    const text = await res.text();
    if (!res.ok) {
      throw new ContextFabricError(res.status, text);
    }
    if (!text) {
      return undefined as T;
    }
    return JSON.parse(text) as T;
  }
}

function enc(s: string): string {
  return encodeURIComponent(s);
}

export function createClient(opts: ClientOptions): ContextFabricClient {
  return new ContextFabricClient(opts);
}
