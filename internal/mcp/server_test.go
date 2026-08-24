package mcp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/adapters/openfga"
	"github.com/xsama/context-fabric/internal/application"
	"github.com/xsama/context-fabric/internal/audit"
	"github.com/xsama/context-fabric/internal/authn"
	"github.com/xsama/context-fabric/internal/mcp"
	"github.com/xsama/context-fabric/internal/policy"
	"github.com/xsama/context-fabric/internal/quota"
	"github.com/xsama/context-fabric/internal/retrieval"
)

func TestToolsListReturnsFiveTools(t *testing.T) {
	store := memory.NewStore()
	idx := memory.NewIndex()
	authz := openfga.NewMemory()
	svc := &app.ApplicationService{
		Identity: authn.NewLocal(),
		Authz:    authz,
		Policy:   policy.New(),
		Ledger:   store,
		Evidence: memory.NewEvidence(),
		Index:    idx,
		Audit:    audit.NewMemory(),
		Quota:    quota.NewLimiter(quota.DefaultLimits()),
		Retrieve: &retrieval.Pipeline{
			Identity: authn.NewLocal(), Authz: authz, Policy: policy.New(),
			Ledger: store, Index: idx, Audit: audit.NewMemory(), Snippets: idx,
		},
	}
	srv := mcp.New(svc)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer local:org1:alice:employee")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Result.Tools) != 5 {
		t.Fatalf("expected 5 tools, got %d: %v", len(resp.Result.Tools), resp.Result.Tools)
	}
	names := map[string]bool{}
	for _, tool := range resp.Result.Tools {
		names[tool["name"].(string)] = true
	}
	for _, want := range []string{"context.search", "context.get", "context.brief", "context.graph", "context.request_access"} {
		if !names[want] {
			t.Fatalf("missing tool %s", want)
		}
	}
}
