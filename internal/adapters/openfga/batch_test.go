package openfga_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xsama/context-fabric/internal/adapters/openfga"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestBatchCheckPutsConsistencyOnRequest(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/batch-check") {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"0":{"allowed":true},"1":{"allowed":false}}}`))
	}))
	t.Cleanup(srv.Close)

	c := &openfga.Client{
		APIURL: srv.URL, StoreID: "store", ModelID: "model",
		HTTPClient: srv.Client(),
	}
	prin := ports.Principal{ID: "u1", Kind: ports.PrincipalKindUser, Subject: "alice", OrgID: "o"}
	out, err := c.BatchCheck(context.Background(), []ports.AuthzCheck{
		{Principal: prin, Action: "can_read", ResourceID: "r1", Consistency: ports.ConsistencyFullyConsistent},
		{Principal: prin, Action: "can_read", ResourceID: "r2", Consistency: ports.ConsistencyMinLatency},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || !out[0].Allowed || out[1].Allowed {
		t.Fatalf("decisions=%+v", out)
	}
	if got["consistency"] != "HIGHER_CONSISTENCY" {
		t.Fatalf("request consistency=%v payload=%v", got["consistency"], got)
	}
	checks, _ := got["checks"].([]any)
	if len(checks) != 2 {
		t.Fatalf("checks=%v", checks)
	}
	for _, raw := range checks {
		item, _ := raw.(map[string]any)
		if _, ok := item["consistency"]; ok {
			t.Fatalf("consistency must not be per-check: %v", item)
		}
	}
}
