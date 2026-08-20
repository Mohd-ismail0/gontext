package openfga_test

import (
	"testing"

	"github.com/xsama/context-fabric/internal/adapters/openfga"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestConsistencyPreferenceMapping(t *testing.T) {
	if openfga.ConsistencyPreference(ports.ConsistencyMinLatency) != "MINIMIZE_LATENCY" {
		t.Fatal("MinLatency mapping")
	}
	if openfga.ConsistencyPreference(ports.ConsistencyFullyConsistent) != "HIGHER_CONSISTENCY" {
		t.Fatal("FullyConsistent mapping")
	}
}
