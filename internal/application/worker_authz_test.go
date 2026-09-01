package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/adapters/openfga"
	app "github.com/xsama/context-fabric/internal/application"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestAuthzOutboxDrainAndReconcile(t *testing.T) {
	ctx := context.Background()
	ledger := memory.NewStore()
	authz := openfga.NewMemory()
	_ = ledger.CreateOrganization(ctx, ports.Organization{ID: "org1", Name: "Org"})

	now := time.Now().UTC()
	_ = ledger.WithOrgTx(ctx, "org1", func(ctx context.Context, tx ports.Tx) error {
		_ = ledger.UpsertRecord(ctx, tx, ports.Record{
			ResourceID: "child", OrgID: "org1", Kind: "message", Title: "c",
			Classification: "internal", State: ports.LifecycleAccepted,
		})
		_ = ledger.UpsertRecord(ctx, tx, ports.Record{
			ResourceID: "parent", OrgID: "org1", Kind: "case", Title: "p",
			Classification: "internal", State: ports.LifecycleAccepted,
		})
		return ledger.UpsertEdge(ctx, tx, ports.GraphEdge{
			EdgeID: "e-parent", OrgID: "org1", FromID: "child", ToID: "parent",
			Predicate: ports.EdgeParent, State: "ACTIVE", SyncAuthz: true, CreatedAt: now, UpdatedAt: now,
		})
	})

	w := &app.Worker{Ledger: ledger, Authz: authz, Batch: 10, MaxAuthzAttempts: 5}
	if err := w.ReconcileAuthz(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _, err := ledger.CountAuthzTuplePending(ctx, "org1")
	if err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("reconcile should enqueue missing tuple, pending=%d", pending)
	}
	if err := w.DrainAuthz(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _, err = ledger.CountAuthzTuplePending(ctx, "org1")
	if err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("drain should clear pending, got %d", pending)
	}
	authz.Grant("resource:parent", "reader", "user:alice")
	dec, err := authz.Check(ctx, ports.AuthzCheck{
		Principal:  ports.Principal{Kind: ports.PrincipalKindUser, Subject: "alice", OrgID: "org1"},
		Action:     "can_read",
		ResourceID: "child",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dec.Allowed {
		t.Fatal("expected parent inheritance after outbox apply")
	}
}

func TestAuthzReconcileReenqueuesAfterOpenFGALoss(t *testing.T) {
	ctx := context.Background()
	ledger := memory.NewStore()
	authz := openfga.NewMemory()
	_ = ledger.CreateOrganization(ctx, ports.Organization{ID: "org1", Name: "Org"})

	now := time.Now().UTC()
	_ = ledger.WithOrgTx(ctx, "org1", func(ctx context.Context, tx ports.Tx) error {
		_ = ledger.UpsertRecord(ctx, tx, ports.Record{
			ResourceID: "child", OrgID: "org1", Kind: "message", Title: "c",
			Classification: "internal", State: ports.LifecycleAccepted,
		})
		_ = ledger.UpsertRecord(ctx, tx, ports.Record{
			ResourceID: "parent", OrgID: "org1", Kind: "case", Title: "p",
			Classification: "internal", State: ports.LifecycleAccepted,
		})
		return ledger.UpsertEdge(ctx, tx, ports.GraphEdge{
			EdgeID: "e-parent", OrgID: "org1", FromID: "child", ToID: "parent",
			Predicate: ports.EdgeParent, State: "ACTIVE", SyncAuthz: true, CreatedAt: now, UpdatedAt: now,
		})
	})

	w := &app.Worker{Ledger: ledger, Authz: authz, Batch: 10, MaxAuthzAttempts: 5}
	if err := w.ReconcileAuthz(ctx); err != nil {
		t.Fatal(err)
	}
	if err := w.DrainAuthz(ctx); err != nil {
		t.Fatal(err)
	}
	// Simulate OpenFGA wipe while outbox still shows applied coverage.
	authz.Revoke("resource:child", "parent", "resource:parent")
	exists, err := authz.HasTuple(ctx, ports.RelationshipTuple{
		Object: "resource:child", Relation: "parent", Subject: "resource:parent",
	})
	if err != nil || exists {
		t.Fatalf("tuple should be gone, exists=%v err=%v", exists, err)
	}
	if err := w.ReconcileAuthz(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _, err := ledger.CountAuthzTuplePending(ctx, "org1")
	if err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("reconcile should re-enqueue after OpenFGA loss, pending=%d", pending)
	}
	if err := w.DrainAuthz(ctx); err != nil {
		t.Fatal(err)
	}
	exists, err = authz.HasTuple(ctx, ports.RelationshipTuple{
		Object: "resource:child", Relation: "parent", Subject: "resource:parent",
	})
	if err != nil || !exists {
		t.Fatalf("tuple should be restored, exists=%v err=%v", exists, err)
	}
}
