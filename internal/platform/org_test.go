package platform_test

import (
	"testing"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestRequireOrgMismatch(t *testing.T) {
	err := platform.RequireOrg(ports.Principal{OrgID: "org_a"}, "org_b")
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	ae, ok := platform.AsAPIError(err)
	if !ok || ae.ReasonCode != "forbidden" {
		t.Fatalf("got %v", err)
	}
	if ae.Message != "organization mismatch" {
		t.Fatalf("message=%q", ae.Message)
	}
}

func TestRequireOrgMissingPrincipalOrg(t *testing.T) {
	err := platform.RequireOrg(ports.Principal{}, "org_a")
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := platform.AsAPIError(err)
	if !ok || ae.ReasonCode != "forbidden" {
		t.Fatalf("got %v", err)
	}
}

func TestRequireOrgEmptyPath(t *testing.T) {
	err := platform.RequireOrg(ports.Principal{OrgID: "org_a"}, "")
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := platform.AsAPIError(err)
	if !ok || ae.ReasonCode != "validation_failed" {
		t.Fatalf("got %v", err)
	}
}

func TestRequireOrgOK(t *testing.T) {
	if err := platform.RequireOrg(ports.Principal{OrgID: "org_a"}, "org_a"); err != nil {
		t.Fatal(err)
	}
}
