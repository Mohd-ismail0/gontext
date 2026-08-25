package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	app "github.com/xsama/context-fabric/internal/application"
)

func TestNewRegistersColonActionRoutes(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("ServeMux rejected RPC-style routes: %v", rec)
		}
	}()
	srv := New(&app.ApplicationService{})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/org1/context/resources/res1:delete", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Fatalf("delete RPC path not routed: %d", rr.Code)
	}
	req2 := httptest.NewRequest(http.MethodPost, "/v1/organizations/org1/agents/ag1/credentials:rotate", nil)
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	if rr2.Code == http.StatusNotFound {
		t.Fatalf("rotate RPC path not routed: %d", rr2.Code)
	}
}

func TestCutRPC(t *testing.T) {
	id, ok := cutRPC("res-1:delete", "delete")
	if !ok || id != "res-1" {
		t.Fatalf("got %q %v", id, ok)
	}
	if _, ok := cutRPC("res-1", "delete"); ok {
		t.Fatal("bare id must not match")
	}
}
