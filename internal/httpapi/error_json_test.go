package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMiddlewareBodyLimitReturnsJSON verifies the 413 raised by the body-limit
// guard uses the JSON error envelope (not text/plain).
func TestMiddlewareBodyLimitReturnsJSON(t *testing.T) {
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}), DefaultMiddlewareConfig())

	body := bytes.Repeat([]byte("a"), (1<<20)+1)
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/org1/context/intake", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected JSON error, got content-type %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "payload_too_large") {
		t.Fatalf("expected reason_code in body: %s", rr.Body.String())
	}
}

// TestMiddlewarePanicReturnsJSON verifies a recovered panic yields a JSON 500.
func TestMiddlewarePanicReturnsJSON(t *testing.T) {
	h := Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), DefaultMiddlewareConfig())

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/org1/context/search", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected JSON error, got content-type %q", ct)
	}
}
