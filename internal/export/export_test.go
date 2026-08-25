package export_test

import (
	"context"
	"strings"
	"testing"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/export"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestExportRoundTripHashStable(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	ev := memory.NewEvidence()
	org := "org_exp_1"
	_ = store.CreateOrganization(ctx, ports.Organization{ID: org, Name: "Exp"})
	_ = store.WithOrgTx(ctx, org, func(ctx context.Context, tx ports.Tx) error {
		_ = store.UpsertRecord(ctx, tx, ports.Record{
			ResourceID: "r1", OrgID: org, Kind: "document", Title: "alpha",
			Classification: "internal", CurrentRevID: "rev1", State: "INDEXED",
		})
		_ = store.AppendRevision(ctx, tx, ports.Revision{
			RevisionID: "rev1", ResourceID: "r1", OrgID: org, State: "INDEXED", ContentHash: "abc",
			EvidenceRef: "org_exp_1/evidence/abc",
		})
		_ = store.UpsertRecord(ctx, tx, ports.Record{
			ResourceID: "r_del", OrgID: org, Kind: "document", Title: "gone",
			Classification: "internal", CurrentRevID: "rev_del", State: "TOMBSTONED",
		})
		_ = store.UpsertEdge(ctx, tx, ports.GraphEdge{
			EdgeID: "e1", OrgID: org, FromID: "r1", ToID: "r1-parent", Predicate: ports.EdgeParent,
			State: "ACTIVE", SyncAuthz: true, Confidence: 1,
		})
		return store.UpsertSource(ctx, tx, ports.SourceRegistration{
			SourceID: "src1", OrgID: org, System: "chatwoot", TrustTier: "verified", Enabled: true,
			Attributes: map[string]string{"api_key": "SHOULD_NOT_EXPORT", "region": "us"},
		})
	})
	_, _ = ev.Put(ctx, "org_exp_1/evidence/abc", strings.NewReader("payload"), "text/plain", nil)

	svc := &export.Service{Ledger: store, Evidence: ev}
	m1, err := svc.Build(ctx, org, "alice")
	if err != nil {
		t.Fatal(err)
	}
	h1 := export.HashManifest(m1)
	m1.Checksums = map[string]string{"manifest_sha256": h1}
	if export.HashManifest(m1) != h1 {
		t.Fatal("hash not stable on recompute")
	}
	for _, src := range m1.Sources {
		if src.Attributes != nil {
			if _, ok := src.Attributes["api_key"]; ok {
				t.Fatal("secrets must not appear in export")
			}
		}
	}

	// Second build of same ledger must produce same content hash.
	m2, err := svc.Build(ctx, org, "bob")
	if err != nil {
		t.Fatal(err)
	}
	h2 := export.HashManifest(m2)
	if h1 != h2 {
		t.Fatalf("export hash not stable: %s vs %s", h1, h2)
	}
	if len(m1.GraphEdges) != 1 || !m1.GraphEdges[0].SyncAuthz {
		t.Fatalf("expected sync_authz edge in export: %#v", m1.GraphEdges)
	}
	if len(m1.AuthzTuples) != 1 || m1.AuthzTuples[0].Relation != "parent" {
		t.Fatalf("expected authz tuple manifest: %#v", m1.AuthzTuples)
	}
	if len(m1.EvidenceRefs) == 0 {
		t.Fatal("expected evidence refs in export")
	}
	if len(m1.Tombstones) == 0 {
		t.Fatal("expected tombstones in export")
	}

	target := "org_isolated_2"
	m1.OrganizationID = target
	for i := range m1.Records {
		m1.Records[i].OrgID = target
	}
	for i := range m1.Revisions {
		m1.Revisions[i].OrgID = target
	}
	for i := range m1.Sources {
		m1.Sources[i].OrgID = target
	}
	m1.Checksums = map[string]string{"manifest_sha256": export.HashManifest(m1)}
	if err := svc.ImportInto(ctx, target, m1); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetRecord(ctx, target, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "alpha" {
		t.Fatalf("import failed: %+v", got)
	}
	edges, err := store.ListEdges(ctx, target, ports.EdgeListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 || edges[0].FromID != "r1" {
		t.Fatalf("imported edges: %#v", edges)
	}
	pending, _, err := store.CountAuthzTuplePending(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("import should enqueue AuthZ tuple, pending=%d", pending)
	}
}

func TestExportHardCapReturnsError(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	org := "org_cap"
	_ = store.CreateOrganization(ctx, ports.Organization{ID: org, Name: "Cap"})
	_ = store.WithOrgTx(ctx, org, func(ctx context.Context, tx ports.Tx) error {
		_ = store.UpsertRecord(ctx, tx, ports.Record{ResourceID: "r1", OrgID: org, Kind: "document", State: "INDEXED"})
		return store.UpsertRecord(ctx, tx, ports.Record{ResourceID: "r2", OrgID: org, Kind: "document", State: "INDEXED"})
	})
	svc := &export.Service{Ledger: store, RecordCap: 1}
	if _, err := svc.Build(ctx, org, "alice"); err == nil {
		t.Fatal("expected bounded-job error")
	}
}
