package mcp

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	app "github.com/xsama/context-fabric/internal/application"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// PRMConfig configures RFC 9728 OAuth protected-resource metadata.
type PRMConfig struct {
	ResourceURL           string
	AuthorizationServers  []string
	ScopesSupported       []string
	ResourceDocumentation string
}

// Server is a read-only MCP adapter over ApplicationService (JSON-RPC over HTTP).
type Server struct {
	App *app.ApplicationService
	PRM PRMConfig
}

// New constructs an MCP HTTP adapter.
func New(svc *app.ApplicationService) *Server {
	return &Server{App: svc, PRM: PRMFromEnv()}
}

// NewWithPRM constructs an MCP adapter with explicit PRM configuration.
func NewWithPRM(svc *app.ApplicationService, prm PRMConfig) *Server {
	return &Server{App: svc, PRM: prm}
}

// PRMFromEnv loads protected-resource metadata from deployment env.
func PRMFromEnv() PRMConfig {
	servers := splitCSV(firstNonEmpty(os.Getenv("MCP_AUTHORIZATION_SERVERS"), os.Getenv("OIDC_ISSUER")))
	scopes := splitCSV(envOr("MCP_SCOPES_SUPPORTED", "context:search,context:read,context:request_access"))
	return PRMConfig{
		ResourceURL:           strings.TrimSpace(os.Getenv("MCP_RESOURCE_URL")),
		AuthorizationServers:  servers,
		ScopesSupported:       scopes,
		ResourceDocumentation: envOr("MCP_RESOURCE_DOCUMENTATION", "https://docs.context-fabric.io/mcp"),
	}
}

// Handler returns the MCP JSON-RPC endpoint handler (POST /mcp).
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.serve)
}

// ProtectedResourceMetadata returns OAuth protected-resource metadata from env (RFC 9728).
func ProtectedResourceMetadata(r *http.Request) map[string]any {
	return (&Server{PRM: PRMFromEnv()}).ProtectedResourceMetadata(r)
}

// ProtectedResourceMetadata returns deployment-configured PRM for this server.
func (s *Server) ProtectedResourceMetadata(r *http.Request) map[string]any {
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	scheme := "https"
	if r.TLS == nil && (strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.0.0.1")) {
		scheme = "http"
	}
	resource := s.PRM.ResourceURL
	if resource == "" {
		resource = scheme + "://" + host + "/mcp"
	}
	authServers := s.PRM.AuthorizationServers
	if len(authServers) == 0 {
		authServers = []string{}
	}
	scopes := s.PRM.ScopesSupported
	if len(scopes) == 0 {
		scopes = []string{"context:search", "context:read", "context:request_access"}
	}
	out := map[string]any{
		"resource":                 resource,
		"authorization_servers":    authServers,
		"scopes_supported":         scopes,
		"bearer_methods_supported": []string{"header"},
	}
	if s.PRM.ResourceDocumentation != "" {
		out["resource_documentation"] = s.PRM.ResourceDocumentation
	}
	return out
}

// WWWAuthenticate builds the RFC 9728 challenge for 401 responses.
func (s *Server) WWWAuthenticate(r *http.Request) string {
	meta := s.ProtectedResourceMetadata(r)
	resource, _ := meta["resource"].(string)
	return `Bearer realm="context-fabric", resource="` + resource + `"`
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

var supportedProtocolVersions = []string{"2024-11-05", "2025-03-26"}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Origin validation for browser-based clients (MCP Streamable HTTP guidance).
	if origin := r.Header.Get("Origin"); origin != "" {
		if !originAllowed(origin) {
			writeUnauthorized(w, r, s, "origin not allowed")
			return
		}
	}
	raw := r.Header.Get("Authorization")
	if strings.TrimSpace(raw) == "" {
		writeUnauthorized(w, r, s, "unauthorized")
		return
	}

	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
		return
	}
	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}

	creds := bearerCreds(r)
	orgID := r.Header.Get("X-Organization-Id")
	if orgID == "" {
		orgID = r.URL.Query().Get("org_id")
	}

	switch req.Method {
	case "initialize":
		proto := "2024-11-05"
		var initParams struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &initParams)
		if initParams.ProtocolVersion != "" {
			for _, v := range supportedProtocolVersions {
				if v == initParams.ProtocolVersion {
					proto = v
					break
				}
			}
		}
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": proto,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "context-fabric", "version": "1.0.0"},
		}})
	case "tools/list", "tools.list":
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": ToolDescriptors()}})
	case "tools/call", "tools.call":
		result, err := s.callTool(r, creds, orgID, req.Params)
		if err != nil {
			writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: toolErrorResult(err)})
			return
		}
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
	default:
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found"}})
	}
}

func writeUnauthorized(w http.ResponseWriter, r *http.Request, s *Server, msg string) {
	if s != nil {
		w.Header().Set("WWW-Authenticate", s.WWWAuthenticate(r))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		Error:   &rpcError{Code: -32001, Message: msg, Data: map[string]any{"reason_code": "unauthorized"}},
	})
}

// ToolDescriptors returns the read-only MCP tools aligned with REST (ADR 0015).
func ToolDescriptors() []map[string]any {
	consistency := map[string]any{"type": "string", "enum": []string{"min_latency", "fully_consistent"}}
	return []map[string]any{
		tool("context.search",
			"Policy-first ranked lexical search (FTS). AuthZ BatchCheck before hydrate. Caps: max_items. Returns cited ContextPacket; may set truncated. Prefer context.get when resource_id is known. Dense/hybrid fusion is out of v1.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organization_id": map[string]any{"type": "string"},
					"query":           map[string]any{"type": "string", "maxLength": 4000},
					"purpose":         map[string]any{"type": "string", "enum": []string{"support", "account_management", "marketing", "finance", "agent_assist"}},
					"scope": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"context_space_id": map[string]any{"type": "string"},
							"case_id":          map[string]any{"type": "string"},
							"brand_id":         map[string]any{"type": "string"},
						},
					},
					"filters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"include_tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
					},
					"max_items":   map[string]any{"type": "integer", "minimum": 1, "maximum": 50},
					"consistency": consistency,
				},
				"required": []string{"purpose"},
			}),
		tool("context.get",
			"Fetch one known resource as a ContextPacket. Use when resource_id is known (routing step 1).",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organization_id": map[string]any{"type": "string"},
					"resource_id":     map[string]any{"type": "string"},
					"purpose":         map[string]any{"type": "string", "enum": []string{"support", "account_management", "marketing", "finance", "agent_assist"}},
					"consistency":     consistency,
				},
				"required": []string{"resource_id", "purpose"},
			}),
		tool("context.brief",
			"Summarized brief for a resource scope. Citations required; never invent facts beyond packet.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organization_id": map[string]any{"type": "string"},
					"resource_id":     map[string]any{"type": "string"},
					"purpose":         map[string]any{"type": "string", "enum": []string{"support", "account_management", "marketing", "finance", "agent_assist"}},
					"max_items":       map[string]any{"type": "integer", "minimum": 1, "maximum": 50},
					"consistency":     consistency,
				},
				"required": []string{"purpose"},
			}),
		tool("context.graph",
			"Visible knowledge subgraph around a seed (AuthZ per node, both edge endpoints). Default depth 1, hard max depth 4, max_nodes 200. next_cursor is truncation signal only—re-query with adjusted caps.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organization_id": map[string]any{"type": "string"},
					"resource_id":     map[string]any{"type": "string"},
					"purpose":         map[string]any{"type": "string", "enum": []string{"support", "account_management", "marketing", "finance", "agent_assist"}},
					"depth":           map[string]any{"type": "integer", "minimum": 0, "maximum": 4, "default": 1},
					"max_nodes":       map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 50},
					"predicates":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"consistency":     consistency,
				},
				"required": []string{"resource_id", "purpose"},
			}),
		tool("context.request_access",
			"Request access when retrieval abstains or AuthZ denies. Prefer after at most three retrieval rounds.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organization_id": map[string]any{"type": "string"},
					"resource_id":     map[string]any{"type": "string"},
					"purpose":         map[string]any{"type": "string", "enum": []string{"support", "account_management", "marketing", "finance", "agent_assist"}},
					"justification":   map[string]any{"type": "string"},
				},
				"required": []string{"resource_id", "purpose"},
			}),
	}
}

func tool(name, desc string, schema map[string]any) map[string]any {
	return map[string]any{
		"name":        name,
		"description": desc,
		"inputSchema": schema,
	}
}

func (s *Server) callTool(r *http.Request, creds ports.Credentials, orgID string, params json.RawMessage) (map[string]any, error) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, platform.ErrValidation("invalid tools/call params")
	}
	if p.Arguments == nil {
		p.Arguments = map[string]any{}
	}
	if v, ok := p.Arguments["organization_id"].(string); ok && v != "" {
		orgID = v
	}
	if orgID == "" {
		return nil, platform.ErrValidation("organization_id required")
	}
	scopes := scopesOf(creds)
	ctx := r.Context()

	switch p.Name {
	case "context.search":
		body := app.SearchRequest{
			Query:       strArg(p.Arguments, "query"),
			Purpose:     strArg(p.Arguments, "purpose"),
			MaxItems:    intArg(p.Arguments, "max_items"),
			Consistency: strArg(p.Arguments, "consistency"),
		}
		if scope, ok := p.Arguments["scope"].(map[string]any); ok {
			body.Scope = scope
		}
		if filters, ok := p.Arguments["filters"].(map[string]any); ok {
			sf := &app.SearchFilters{}
			if tags, ok := filters["include_tags"].([]any); ok {
				for _, t := range tags {
					if s, ok := t.(string); ok {
						sf.IncludeTags = append(sf.IncludeTags, s)
					}
				}
			}
			body.Filters = sf
		}
		pkt, err := s.App.Search(ctx, creds, orgID, scopes, body)
		if err != nil {
			return nil, err
		}
		return toolResult(pkt)
	case "context.get":
		pkt, err := s.App.GetResource(ctx, creds, orgID, strArg(p.Arguments, "resource_id"), strArg(p.Arguments, "purpose"), strArg(p.Arguments, "consistency"), scopes)
		if err != nil {
			return nil, err
		}
		return toolResult(pkt)
	case "context.brief":
		pkt, err := s.App.Brief(ctx, creds, orgID, scopes, strArg(p.Arguments, "purpose"), strArg(p.Arguments, "resource_id"), strArg(p.Arguments, "consistency"), intArg(p.Arguments, "max_items"))
		if err != nil {
			return nil, err
		}
		return toolResult(pkt)
	case "context.graph":
		preds := strSliceArg(p.Arguments, "predicates")
		pkt, err := s.App.Graph(ctx, creds, orgID, scopes, app.GraphRequest{
			ResourceID:  strArg(p.Arguments, "resource_id"),
			Purpose:     strArg(p.Arguments, "purpose"),
			Depth:       intArg(p.Arguments, "depth"),
			MaxNodes:    intArg(p.Arguments, "max_nodes"),
			Predicates:  preds,
			Consistency: strArg(p.Arguments, "consistency"),
		})
		if err != nil {
			return nil, err
		}
		return toolResult(pkt)
	case "context.request_access":
		out, err := s.App.RequestAccess(ctx, creds, orgID, strArg(p.Arguments, "resource_id"), strArg(p.Arguments, "purpose"), strArg(p.Arguments, "justification"))
		if err != nil {
			return nil, err
		}
		return toolResult(out)
	default:
		return nil, platform.ErrValidation("unknown tool")
	}
}

func toolResult(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(raw)}},
		"isError": false,
	}, nil
}

func toolErrorResult(err error) map[string]any {
	ae, ok := platform.AsAPIError(err)
	reason := "internal_error"
	msg := err.Error()
	retryable := false
	auditID := ""
	var retryAfter int
	if ok {
		reason = ae.ReasonCode
		msg = ae.Message
		retryable = ae.Retryable
		auditID = ae.AuditID
		if ae.RetryAfter > 0 {
			retryAfter = int(ae.RetryAfter / time.Second)
		}
	}
	payload := map[string]any{
		"reason_code": reason,
		"message":     msg,
		"retryable":   retryable,
		"audit_id":    auditID,
	}
	if retryAfter > 0 {
		payload["retry_after_seconds"] = retryAfter
	}
	raw, _ := json.Marshal(payload)
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(raw)}},
		"isError": true,
	}
}

func bearerCreds(r *http.Request) ports.Credentials {
	raw := r.Header.Get("Authorization")
	token := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	creds := ports.Credentials{BearerToken: token}
	if os.Getenv("CONTEXT_FABRIC_ALLOW_SCOPE_HEADER") == "1" {
		scopes := strings.Fields(r.Header.Get("X-Context-Scopes"))
		if len(scopes) > 0 {
			creds.Extra = map[string]string{"scopes": strings.Join(scopes, " ")}
		}
	}
	return creds
}

func scopesOf(creds ports.Credentials) []string {
	if creds.Extra == nil {
		return nil
	}
	return strings.Fields(creds.Extra["scopes"])
}

func originAllowed(origin string) bool {
	allow := strings.TrimSpace(os.Getenv("MCP_ALLOWED_ORIGINS"))
	if allow == "" || allow == "*" {
		return true
	}
	for _, o := range splitCSV(allow) {
		if strings.EqualFold(o, origin) {
			return true
		}
	}
	return false
}

func strArg(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

func intArg(m map[string]any, k string) int {
	switch v := m[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func strSliceArg(m map[string]any, k string) []string {
	raw, ok := m[k]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func writeRPC(w http.ResponseWriter, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
