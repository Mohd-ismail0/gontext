package memory_test

import (
	"context"
	"testing"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestRevokeIsOrgScoped(t *testing.T) {
	ctx := context.Background()
	store := memory.NewCredentialStore()
	a, err := store.CreateAgentCredential(ctx, ports.CreateAgentCredentialRequest{OrgID: "org-a", AgentID: "ag"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(ctx, "org-b", a.CredentialID); err == nil {
		t.Fatal("cross-tenant revoke must fail")
	}
	if _, err := store.ResolveAPIKey(ctx, a.Secret); err != nil {
		t.Fatal("key still valid after foreign revoke attempt")
	}
	if err := store.Revoke(ctx, "org-a", a.CredentialID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveAPIKey(ctx, a.Secret); err == nil {
		t.Fatal("expected revoked")
	}
}
