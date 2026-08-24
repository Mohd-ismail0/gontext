package retrieval_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/adapters/openfga"
	"github.com/xsama/context-fabric/internal/audit"
	"github.com/xsama/context-fabric/internal/authn"
	"github.com/xsama/context-fabric/internal/policy"
	"github.com/xsama/context-fabric/internal/ports"
	"github.com/xsama/context-fabric/internal/retrieval"
)

func TestGraphBoundedCyclicGraph(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	authz := openfga.NewMemory()
	org := "org_cycle"
	_ = store.CreateOrganization(ctx, ports.Organization{ID: org, Name: "Cycle"})

	const n = 8
	_ = store.WithOrgTx(ctx, org, func(ctx context.Context, tx ports.Tx) error {
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("n%d", i)
			if err := store.UpsertRecord(ctx, tx, ports.Record{
				ResourceID: id, OrgID: org, Kind: "node", Title: id,
				Classification: "internal", CurrentRevID: "r1", State: "INDEXED",
			}); err != nil {
				return err
			}
			authz.Grant("resource:"+id, "reader", "user:alice")
			next := fmt.Sprintf("n%d", (i+1)%n)
			if err := store.UpsertEdge(ctx, tx, ports.GraphEdge{
				EdgeID: fmt.Sprintf("e%d", i), OrgID: org, FromID: id, ToID: next,
				Predicate: ports.EdgeRelatedTo, State: "ACTIVE",
			}); err != nil {
				return err
			}
		}
		return nil
	})
	authz.AddOrgMember(org, "alice")

	pipe := &retrieval.Pipeline{
		Identity: authn.NewLocal(),
		Authz:    authz,
		Policy:   policy.New(),
		Ledger:   store,
		Index:    memory.NewIndex(),
		Audit:    audit.NewMemory(),
	}
	pkt, err := pipe.Graph(ctx, retrieval.Request{
		Credentials: ports.Credentials{BearerToken: "local:org_cycle:alice:employee"},
		OrgID:       org,
		Purpose:     "support",
		ResourceID:  "n0",
		Depth:       3,
		MaxNodes:    5,
		Action:      "context.graph",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt.Nodes) > 5 {
		t.Fatalf("max_nodes not respected: %d", len(pkt.Nodes))
	}
	if !pkt.Truncated {
		t.Fatal("expected truncated when max_nodes < cycle size")
	}
	seedSeen := false
	for _, n := range pkt.Nodes {
		if n.ResourceID == "n0" {
			seedSeen = true
			break
		}
	}
	if !seedSeen {
		t.Fatal("seed node must be retained when max_nodes truncates")
	}
	for _, e := range pkt.Edges {
		seen := map[string]bool{}
		for _, n := range pkt.Nodes {
			seen[n.ResourceID] = true
		}
		if !seen[e.FromID] || !seen[e.ToID] {
			t.Fatalf("dangling edge after truncation: %#v", e)
		}
	}
}
