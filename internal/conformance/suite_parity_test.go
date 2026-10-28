package conformance

import "testing"

// TestEverySuiteCaseHasHandler guards against silent drift between
// contracts/conformance/suite.yaml and the in-process runner: the runner
// dispatches by case ID, so a case added to YAML without a handler would
// otherwise be skipped without notice.
func TestEverySuiteCaseHasHandler(t *testing.T) {
	suite, err := LoadSuite(DefaultSuitePath())
	if err != nil {
		t.Fatal(err)
	}
	if len(suite.Cases) == 0 {
		t.Fatal("suite.yaml contains no cases")
	}
	for _, c := range suite.Cases {
		if _, ok := caseHandlers[c.ID]; !ok {
			t.Errorf("suite.yaml case %q has no in-process handler", c.ID)
		}
	}
}
