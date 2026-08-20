package authn_test

import (
	"context"
	"testing"

	"github.com/xsama/context-fabric/internal/authn"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestLocalAuthenticate(t *testing.T) {
	p := authn.NewLocal()
	prin, err := p.Authenticate(context.Background(), ports.Credentials{
		BearerToken: "Bearer local:acme:alice:admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if prin.OrgID != "acme" || prin.Subject != "alice" {
		t.Fatalf("got %#v", prin)
	}
	if prin.Kind != ports.PrincipalKindUser {
		t.Fatalf("kind=%s", prin.Kind)
	}
	if len(prin.Roles) != 1 || prin.Roles[0] != "admin" {
		t.Fatalf("roles=%v", prin.Roles)
	}
	if prin.Issuer != "local" || prin.ID != "local|alice" {
		t.Fatalf("identity %#v", prin)
	}
	if len(prin.Scopes) == 0 {
		t.Fatal("admin should receive role-derived scopes")
	}
	found := false
	for _, s := range prin.Scopes {
		if s == "context:manage_policy" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("admin scopes missing manage_policy: %v", prin.Scopes)
	}
}

func TestLocalAgentRole(t *testing.T) {
	p := authn.NewLocal()
	prin, err := p.Authenticate(context.Background(), ports.Credentials{
		BearerToken: "local:org1:bot1:agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if prin.Kind != ports.PrincipalKindAgent {
		t.Fatalf("kind=%s", prin.Kind)
	}
}

func TestLocalRejectsMalformed(t *testing.T) {
	p := authn.NewLocal()
	_, err := p.Authenticate(context.Background(), ports.Credentials{BearerToken: "local:only-two"})
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := platform.AsAPIError(err)
	if !ok || ae.ReasonCode != "unauthorized" {
		t.Fatalf("got %v", err)
	}
}

func TestLocalDiscover(t *testing.T) {
	p := authn.NewLocal()
	meta, err := p.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if meta.Issuer != "local" {
		t.Fatalf("issuer=%q", meta.Issuer)
	}
}
