package platform_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/xsama/context-fabric/internal/platform"
)

func TestAPIErrorHelpers(t *testing.T) {
	cases := []struct {
		name   string
		err    *platform.APIError
		status int
		code   string
		retry  bool
	}{
		{"unauthorized", platform.ErrUnauthorized("nope"), http.StatusUnauthorized, "unauthorized", false},
		{"forbidden", platform.ErrForbidden("denied"), http.StatusForbidden, "forbidden", false},
		{"not_found", platform.ErrNotFound("missing"), http.StatusNotFound, "not_found", false},
		{"conflict", platform.ErrConflict("dup"), http.StatusConflict, "conflict", false},
		{"validation", platform.ErrValidation("bad"), http.StatusBadRequest, "validation_failed", false},
		{"rate", platform.ErrRateLimited("slow", 2*time.Second), http.StatusTooManyRequests, "rate_limited", true},
		{"unavailable", platform.ErrUnavailable("down"), http.StatusServiceUnavailable, "unavailable", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.HTTPStatus != tc.status {
				t.Fatalf("status=%d want %d", tc.err.HTTPStatus, tc.status)
			}
			if tc.err.ReasonCode != tc.code {
				t.Fatalf("reason=%q want %q", tc.err.ReasonCode, tc.code)
			}
			if tc.err.Retryable != tc.retry {
				t.Fatalf("retryable=%v want %v", tc.err.Retryable, tc.retry)
			}
			if tc.err.Message == "" {
				t.Fatal("expected message")
			}
			if tc.err.Error() == "" {
				t.Fatal("expected Error() string")
			}
			var ae *platform.APIError
			if !errors.As(tc.err, &ae) {
				t.Fatal("errors.As failed")
			}
		})
	}
}

func TestErrRateLimitedRetryAfter(t *testing.T) {
	err := platform.ErrRateLimited("quota", 1500*time.Millisecond)
	if err.RetryAfter != 1500*time.Millisecond {
		t.Fatalf("RetryAfter=%v", err.RetryAfter)
	}
}

func TestNewIDs(t *testing.T) {
	seen := map[string]struct{}{}
	ids := []string{
		platform.NewEventID(),
		platform.NewResourceID(),
		platform.NewRevisionID(),
		platform.NewArtifactID(),
	}
	for _, id := range ids {
		if id == "" {
			t.Fatal("empty id")
		}
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id %s", id)
		}
		seen[id] = struct{}{}
	}
}
