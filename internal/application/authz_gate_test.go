package app

import (
	"context"
	"testing"

	"github.com/xsama/context-fabric/internal/adapters/openfga"
	"github.com/xsama/context-fabric/internal/authn"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestRequireManageDeniedWithoutScope(t *testing.T) {
	svc := &ApplicationService{
		Identity: authn.NewLocal(),
		Authz:    openfga.NewMemory(),
	}
	org := "org_authz_gate"
	authz := openfga.NewMemory()
	authz.AddOrgMember(org, "bob")
	svc.Authz = authz
	principal := ports.Principal{
		ID: "local|bob", OrgID: org, Subject: "bob",
		Scopes: []string{ScopeSearch, ScopeRead},
	}
	err := svc.requireManage(context.Background(), principal, org, nil)
	if err == nil {
		t.Fatal("expected forbidden without manage scope")
	}
}

func TestRequireIngestNeedsScope(t *testing.T) {
	svc := &ApplicationService{Identity: authn.NewLocal()}
	principal := ports.Principal{OrgID: "org1", Subject: "alice", Scopes: []string{ScopeSearch}}
	err := svc.requireIngest(context.Background(), principal, "org1", nil)
	if err == nil {
		t.Fatal("expected forbidden without ingest scope")
	}
}
