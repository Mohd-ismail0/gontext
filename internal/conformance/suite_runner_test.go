package conformance_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xsama/context-fabric/internal/conformance"
)

func TestConformanceSuiteInProcess(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	suitePath := filepath.Join(filepath.Dir(file), "..", "..", "contracts", "conformance", "suite.yaml")
	rep, err := conformance.Run(context.Background(), conformance.RunOptions{SuitePath: suitePath})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) == 0 {
		t.Fatal("no results")
	}
	var failed int
	for _, r := range rep.Results {
		name := r.ID
		switch {
		case r.Skipped:
			t.Logf("SKIP %s: %s", name, r.Detail)
		case r.Passed:
			t.Logf("PASS %s", name)
		default:
			failed++
			t.Errorf("FAIL %s: %s", name, r.Error)
		}
	}
	if failed > 0 {
		t.Fatalf("%d conformance case(s) failed", failed)
	}
}
