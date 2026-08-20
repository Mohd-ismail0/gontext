package memory

import (
	"context"
	"testing"
	"time"

	"github.com/xsama/context-fabric/internal/ports"
)

func TestListEdgesOrdersBeforeApplyingLimit(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	orgID := "org_edges"
	if err := store.CreateOrganization(ctx, ports.Organization{ID: orgID, Name: "Edges"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	if err := store.WithOrgTx(ctx, orgID, func(ctx context.Context, tx ports.Tx) error {
		for _, edge := range []ports.GraphEdge{
			{EdgeID: "edge-z", OrgID: orgID, FromID: "z", ToID: "a", Predicate: ports.EdgeMentions, State: "ACTIVE", CreatedAt: now},
			{EdgeID: "edge-a", OrgID: orgID, FromID: "a", ToID: "z", Predicate: ports.EdgeMentions, State: "ACTIVE", CreatedAt: now},
			{EdgeID: "edge-b", OrgID: orgID, FromID: "b", ToID: "a", Predicate: ports.EdgeMentions, State: "ACTIVE", CreatedAt: now},
		} {
			if err := store.UpsertEdge(ctx, tx, edge); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	edges, err := store.ListEdges(ctx, orgID, ports.EdgeListOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 2 {
		t.Fatalf("got %d edges, want 2", len(edges))
	}
	if edges[0].EdgeID != "edge-a" || edges[1].EdgeID != "edge-b" {
		t.Fatalf("got order %q, %q; want edge-a, edge-b", edges[0].EdgeID, edges[1].EdgeID)
	}
}
