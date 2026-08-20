package authzsync_test

import (
	"context"
	"testing"
	"time"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/authzsync"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestEnqueueForEdgeCreatesParentTupleOperation(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	orgID := "org_authz_sync"
	if err := store.CreateOrganization(ctx, ports.Organization{ID: orgID, Name: "AuthZ Sync"}); err != nil {
		t.Fatal(err)
	}

	edge := ports.GraphEdge{
		EdgeID: "edge_parent", OrgID: orgID, FromID: "child", ToID: "parent",
		Predicate: ports.EdgeParent, State: "ACTIVE", SyncAuthz: true,
	}
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	if err := store.WithOrgTx(ctx, orgID, func(ctx context.Context, tx ports.Tx) error {
		return authzsync.EnqueueForEdge(ctx, store, tx, edge, authzsync.OperationWrite, now)
	}); err != nil {
		t.Fatal(err)
	}

	pending, dead, err := store.CountAuthzTuplePending(ctx, orgID)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 1 || dead != 0 {
		t.Fatalf("pending=%d dead=%d, want 1 and 0", pending, dead)
	}
	if ok, err := store.HasAuthzTupleCoverage(ctx, orgID, authzsync.OperationWrite, "resource:child", ports.EdgeParent, "resource:parent"); err != nil || !ok {
		t.Fatalf("expected tuple coverage, ok=%v err=%v", ok, err)
	}
}

func TestEnqueueForEdgeIgnoresKnowledgeEdgesWithoutInheritance(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	orgID := "org_authz_sync"
	if err := store.CreateOrganization(ctx, ports.Organization{ID: orgID, Name: "AuthZ Sync"}); err != nil {
		t.Fatal(err)
	}

	edge := ports.GraphEdge{EdgeID: "edge_related", OrgID: orgID, FromID: "a", ToID: "b", Predicate: ports.EdgeRelatedTo}
	if err := store.WithOrgTx(ctx, orgID, func(ctx context.Context, tx ports.Tx) error {
		return authzsync.EnqueueForEdge(ctx, store, tx, edge, authzsync.OperationWrite, time.Now().UTC())
	}); err != nil {
		t.Fatal(err)
	}
	pending, _, err := store.CountAuthzTuplePending(ctx, orgID)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("knowledge-only edge must not enqueue AuthZ work; pending=%d", pending)
	}
}
