package authn_test

import (
	"context"
	"testing"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/authn"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestChainedAPIKeyBearerAuthenticates(t *testing.T) {
	store := memory.NewCredentialStore()
	cred, err := store.CreateAgentCredential(context.Background(), ports.CreateAgentCredentialRequest{
		OrgID: "org-a", AgentID: "agent-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	idp := authn.WithAPIKeys(authn.NewLocal(), store)
	prin, err := idp.Authenticate(context.Background(), ports.Credentials{BearerToken: cred.Secret})
	if err != nil {
		t.Fatal(err)
	}
	if prin.OrgID != "org-a" || prin.Kind != ports.PrincipalKindAgent {
		t.Fatalf("principal=%+v", prin)
	}
	if len(prin.Scopes) == 0 {
		t.Fatal("agent scopes required for retrieval gate")
	}
}

func TestChainedAPIKeyHeaderField(t *testing.T) {
	store := memory.NewCredentialStore()
	cred, err := store.CreateAgentCredential(context.Background(), ports.CreateAgentCredentialRequest{
		OrgID: "org-a", AgentID: "agent-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	idp := authn.WithAPIKeys(authn.NewLocal(), store)
	prin, err := idp.Authenticate(context.Background(), ports.Credentials{APIKey: cred.Secret})
	if err != nil {
		t.Fatal(err)
	}
	if prin.Subject != "agent-2" {
		t.Fatalf("subject=%s", prin.Subject)
	}
}

func TestChainedLocalTokenStillWorks(t *testing.T) {
	idp := authn.WithAPIKeys(authn.NewLocal(), memory.NewCredentialStore())
	prin, err := idp.Authenticate(context.Background(), ports.Credentials{
		BearerToken: "local:org1:alice:employee",
	})
	if err != nil {
		t.Fatal(err)
	}
	if prin.Subject != "alice" {
		t.Fatalf("subject=%s", prin.Subject)
	}
}

func TestChainedRejectsUnknownKey(t *testing.T) {
	idp := authn.WithAPIKeys(authn.NewLocal(), memory.NewCredentialStore())
	if _, err := idp.Authenticate(context.Background(), ports.Credentials{APIKey: "nope"}); err == nil {
		t.Fatal("expected unauthorized")
	}
}
