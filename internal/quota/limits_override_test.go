package quota

import "testing"

// TestLimiterPerOrgOverride verifies persisted per-org limits take effect: an org
// with a larger burst is allowed more immediate requests than a default org.
func TestLimiterPerOrgOverride(t *testing.T) {
	l := NewLimiter(Limits{SearchPerMinute: 60, Burst: 1})
	l.SetOrgLimits("big", Limits{SearchPerMinute: 6000, Burst: 5})

	big := Key{OrgID: "big", Op: OpSearch}
	for i := 0; i < 5; i++ {
		if err := l.Allow(big); err != nil {
			t.Fatalf("big org allow %d failed: %v", i+1, err)
		}
	}
	if err := l.Allow(big); err == nil {
		t.Fatal("expected big org burst to be exhausted on the 6th call")
	}

	small := Key{OrgID: "small", Op: OpSearch}
	if err := l.Allow(small); err != nil {
		t.Fatal(err)
	}
	if err := l.Allow(small); err == nil {
		t.Fatal("expected default burst=1 org to exhaust on the second call")
	}
}
