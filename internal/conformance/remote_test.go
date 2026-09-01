package conformance_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/adapters/openfga"
	app "github.com/xsama/context-fabric/internal/application"
	"github.com/xsama/context-fabric/internal/audit"
	"github.com/xsama/context-fabric/internal/authn"
	"github.com/xsama/context-fabric/internal/conformance"
	"github.com/xsama/context-fabric/internal/httpapi"
	"github.com/xsama/context-fabric/internal/policy"
	"github.com/xsama/context-fabric/internal/quota"
	"github.com/xsama/context-fabric/internal/retrieval"
)

func TestRemoteRunnerToolsCallAndParity(t *testing.T) {
	store := memory.NewStore()
	idx := memory.NewIndex()
	authz := openfga.NewMemory()
	identity := authn.NewLocal()
	svc := &app.ApplicationService{
		Identity: identity,
		Authz:    authz,
		Policy:   policy.New(),
		Ledger:   store,
		Evidence: memory.NewEvidence(),
		Index:    idx,
		Audit:    audit.NewMemory(),
		Quota:    quota.NewLimiter(quota.DefaultLimits()),
		Ready:    func() bool { return true },
		ReadyDetail: func() (bool, map[string]any) {
			return true, map[string]any{"process": map[string]any{"ok": true}}
		},
		Build: app.VersionInfo{AuthzModelID: "test-model"},
		Retrieve: &retrieval.Pipeline{
			Identity: identity, Authz: authz, Policy: policy.New(),
			Ledger: store, Index: idx, Audit: audit.NewMemory(), Snippets: idx,
		},
	}
	ts := httptest.NewServer(httpapi.New(svc).Handler())
	t.Cleanup(ts.Close)

	runner := conformance.NewRemoteRunner(conformance.RemoteOptions{
		BaseURL: ts.URL,
		Token:   "local:org1:alice:employee",
		OrgID:   "org1",
	})
	rep, err := runner.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]conformance.RemoteCheck{}
	for _, c := range rep.Checks {
		byName[c.Name] = c
		if !c.Skipped && !c.Passed {
			t.Errorf("%s failed: %s", c.Name, c.Error)
		}
	}
	for _, name := range []string{
		"health.ready", "prm.oauth-protected-resource", "mcp.unauthorized",
		"mcp.initialize", "mcp.tools/list", "mcp.tools/call",
		"rest.context.search", "mcp.rest.parity",
	} {
		c, ok := byName[name]
		if !ok {
			t.Fatalf("missing check %s", name)
		}
		if c.Skipped || !c.Passed {
			t.Fatalf("%s not passed: skipped=%v err=%s", name, c.Skipped, c.Error)
		}
	}
}
