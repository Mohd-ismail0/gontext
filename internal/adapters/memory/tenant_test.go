package memory_test

import (
	"context"
	"testing"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestTenantIsolation(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	_ = store.CreateOrganization(ctx, ports.Organization{ID: "orgA", Name: "A"})
	_ = store.CreateOrganization(ctx, ports.Organization{ID: "orgB", Name: "B"})

	recA := ports.Record{ResourceID: "r1", OrgID: "orgA", Kind: "document", Title: "secret-a", Classification: "internal"}
	recB := ports.Record{ResourceID: "r1", OrgID: "orgB", Kind: "document", Title: "secret-b", Classification: "internal"}

	_ = store.WithOrgTx(ctx, "orgA", func(ctx context.Context, tx ports.Tx) error {
		return store.UpsertRecord(ctx, tx, recA)
	})
	_ = store.WithOrgTx(ctx, "orgB", func(ctx context.Context, tx ports.Tx) error {
		return store.UpsertRecord(ctx, tx, recB)
	})

	gotA, err := store.GetRecord(ctx, "orgA", "r1")
	if err != nil {
		t.Fatal(err)
	}
	gotB, err := store.GetRecord(ctx, "orgB", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if gotA.Title != "secret-a" || gotB.Title != "secret-b" {
		t.Fatalf("tenant leak or overwrite: a=%q b=%q", gotA.Title, gotB.Title)
	}

	idx := memory.NewIndex()
	_ = idx.Upsert(ctx, []ports.IndexDocument{
		{OrgID: "orgA", ResourceID: "r1", Text: "alpha only", Attributes: map[string]string{"classification": "internal", "purpose_allowlist": "support"}, Labels: []string{"purpose:support"}},
		{OrgID: "orgB", ResourceID: "r1", Text: "beta only", Attributes: map[string]string{"classification": "internal", "purpose_allowlist": "support"}, Labels: []string{"purpose:support"}},
	})
	hitsA, err := idx.SearchCandidates(ctx, "orgA", "alpha", 10, map[string]string{"purpose": "support"})
	if err != nil {
		t.Fatal(err)
	}
	hitsB, err := idx.SearchCandidates(ctx, "orgB", "alpha", 10, map[string]string{"purpose": "support"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hitsA) != 1 {
		t.Fatalf("orgA expected 1 hit, got %d", len(hitsA))
	}
	if len(hitsB) != 0 {
		t.Fatalf("orgB must not see orgA text; got %d hits", len(hitsB))
	}
}
