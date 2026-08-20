package mcp

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/xsama/context-fabric/internal/application"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// Server is a read-only MCP adapter over ApplicationService (JSON-RPC over HTTP).
type Server struct {
	App *app.ApplicationService
}

// New constructs an MCP HTTP adapter.
func New(svc *app.ApplicationService) *Server {
	return &Server{App: svc}
}

// Handler returns the MCP JSON-RPC endpoint handler (POST /mcp).
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.serve)
}

// ProtectedResourceMetadata returns OAuth protected-resource metadata for MCP.
func ProtectedResourceMetadata(r *http.Request) map[string]any {
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	scheme := "https"
	if r.TLS == nil && strings.HasPrefix(r.Host, "localhost") {
		scheme = "http"
	}
	return map[string]any{
		"resource":                 scheme + "://" + host + "/mcp",
		"authorization_servers":    []string{"https://auth.example.com"},
		"scopes_supported":         []string{"context:search", "context:read", "context:request_access"},
		"bearer_methods_supported": []string{"header"},
	}
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
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw := r.Header.Get("Authorization")
	if strings.TrimSpace(raw) == "" {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32001, Message: "unauthorized"}})
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
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "context-fabric", "version": "1.0.0"},
		}})
	case "tools/list", "tools.list":
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": ToolDescriptors()}})
	case "tools/call", "tools.call":
		result, err := s.callTool(r, creds, orgID, req.Params)
		if err != nil {
			writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: err.Error()}})
			return
		}
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
	default:
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found"}})
	}
}

// ToolDescriptors returns the four read-only MCP tools.
func ToolDescriptors() []map[string]any {
	return []map[string]any{
		tool("context.search", "Governed hybrid search returning a ContextPacket", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"organization_id": map[string]any{"type": "string"},
				"query":           map[string]any{"type": "string"},
				"purpose":         map[string]any{"type": "string"},
				"max_items":       map[string]any{"type": "integer"},
			},
			"required": []string{"purpose"},
		}),
		tool("context.get", "Fetch one resource as a ContextPacket", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"organization_id": map[string]any{"type": "string"},
				"resource_id":     map[string]any{"type": "string"},
				"purpose":         map[string]any{"type": "string"},
			},
			"required": []string{"resource_id", "purpose"},
		}),
		tool("context.brief", "Summarized brief for a resource scope", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"organization_id": map[string]any{"type": "string"},
				"resource_id":     map[string]any{"type": "string"},
				"purpose":         map[string]any{"type": "string"},
				"max_items":       map[string]any{"type": "integer"},
			},
			"required": []string{"purpose"},
		}),
		tool("context.request_access", "Request access to a resource", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"organization_id": map[string]any{"type": "string"},
				"resource_id":     map[string]any{"type": "string"},
				"purpose":         map[string]any{"type": "string"},
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
			Query:    strArg(p.Arguments, "query"),
			Purpose:  strArg(p.Arguments, "purpose"),
			MaxItems: intArg(p.Arguments, "max_items"),
		}
		pkt, err := s.App.Search(ctx, creds, orgID, scopes, body)
		if err != nil {
			return nil, err
		}
		return toolResult(pkt)
	case "context.get":
		pkt, err := s.App.GetResource(ctx, creds, orgID, strArg(p.Arguments, "resource_id"), strArg(p.Arguments, "purpose"), "", scopes)
		if err != nil {
			return nil, err
		}
		return toolResult(pkt)
	case "context.brief":
		pkt, err := s.App.Brief(ctx, creds, orgID, scopes, strArg(p.Arguments, "purpose"), strArg(p.Arguments, "resource_id"), "", intArg(p.Arguments, "max_items"))
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

func writeRPC(w http.ResponseWriter, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
