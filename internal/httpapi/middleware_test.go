package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	app "github.com/xsama/context-fabric/internal/application"
)

func TestMiddlewareSetsSecurityHeaders(t *testing.T) {
	srv := New(&app.ApplicationService{})
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing security header")
	}
	if rr.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected request id")
	}
}

func TestMiddlewareIntakeBatchBodyLimit(t *testing.T) {
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.Copy(io.Discard, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}), DefaultMiddlewareConfig())
	body := bytes.Repeat([]byte("a"), (8<<20)+1)
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/org1/context/intake:batch", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rr.Code)
	}
}
