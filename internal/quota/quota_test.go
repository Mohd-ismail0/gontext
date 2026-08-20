package quota_test

import (
	"testing"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/quota"
)

func TestLimiterRateLimits(t *testing.T) {
	l := quota.NewLimiter(quota.Limits{
		SearchPerMinute: 60,
		IntakePerMinute: 60,
		ExportPerMinute: 60,
		Burst:           2,
	})
	key := quota.Key{OrgID: "o1", PrincipalID: "p1", Op: quota.OpSearch}
	if err := l.Allow(key); err != nil {
		t.Fatal(err)
	}
	if err := l.Allow(key); err != nil {
		t.Fatal(err)
	}
	err := l.Allow(key)
	if err == nil {
		t.Fatal("expected rate limit")
	}
	ae, ok := platform.AsAPIError(err)
	if !ok {
		t.Fatalf("want APIError, got %T", err)
	}
	if ae.ReasonCode != "rate_limited" || ae.HTTPStatus != 429 {
		t.Fatalf("got %#v", ae)
	}
	if ae.RetryAfter <= 0 {
		t.Fatalf("expected RetryAfter, got %v", ae.RetryAfter)
	}
}

func TestLimiterScopesSeparately(t *testing.T) {
	l := quota.NewLimiter(quota.Limits{SearchPerMinute: 10, Burst: 1})
	a := quota.Key{OrgID: "o1", PrincipalID: "p1", Op: quota.OpSearch}
	b := quota.Key{OrgID: "o1", PrincipalID: "p2", Op: quota.OpSearch}
	if err := l.Allow(a); err != nil {
		t.Fatal(err)
	}
	if err := l.Allow(b); err != nil {
		t.Fatal(err)
	}
	if err := l.Allow(a); err == nil {
		t.Fatal("expected a limited")
	}
}

func TestLimiterOperations(t *testing.T) {
	l := quota.NewLimiter(quota.Limits{
		SearchPerMinute: 100,
		IntakePerMinute: 100,
		ExportPerMinute: 100,
		Burst:           1,
	})
	for _, op := range []quota.Operation{quota.OpSearch, quota.OpIntake, quota.OpExport} {
		key := quota.Key{OrgID: "o", SourceID: "s", PrincipalID: "p", Op: op}
		if err := l.Allow(key); err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		if err := l.Allow(key); err == nil {
			t.Fatalf("%s: expected limit", op)
		}
	}
}
