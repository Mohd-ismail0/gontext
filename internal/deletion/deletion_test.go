package deletion_test

import (
	"context"
	"testing"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/adapters/openfga"
	"github.com/xsama/context-fabric/internal/audit"
	"github.com/xsama/context-fabric/internal/authn"
	"github.com/xsama/context-fabric/internal/deletion"
	"github.com/xsama/context-fabric/internal/policy"
	"github.com/xsama/context-fabric/internal/ports"
	"github.com/xsama/context-fabric/internal/retrieval"
)

func TestTombstoneDominatesSearch(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	idx := memory.NewIndex()
	authz := openfga.NewMemory()
	org := "org_del_1"
	_ = store.CreateOrganization(ctx, ports.Organization{ID: org, Name: "Del"})

	rec := ports.Record{
		ResourceID: "res_secret", OrgID: org, Kind: "document",
		Title: "delete-me secret", Classification: "internal",
		CurrentRevID: "rev1", State: "INDEXED",
	}
	_ = store.WithOrgTx(ctx, org, func(ctx context.Context, tx ports.Tx) error {
		_ = store.UpsertRecord(ctx, tx, rec)
		return store.AppendRevision(ctx, tx, ports.Revision{
			RevisionID: "rev1", ResourceID: rec.ResourceID, OrgID: org, State: "INDEXED",
			EvidenceRef: "ev/key#v1",
		})
	})
	_ = idx.Upsert(ctx, []ports.IndexDocument{{
		ResourceID: rec.ResourceID, RevisionID: "rev1", OrgID: org,
		Text: "delete-me secret billing", Labels: []string{"purpose:support"},
		Attributes: map[string]string{"classification": "internal", "purpose_allowlist": "support"},
	}})
	authz.AddOrgMember(org, "alice")
	authz.Grant("resource:res_secret", "can_read", "user:alice")
	authz.Grant("resource:res_secret", "owner", "user:alice")

	pipe := &retrieval.Pipeline{
		Identity: authn.NewLocal(), Authz: authz, Policy: policy.New(),
		Ledger: store, Index: idx, Audit: audit.NewMemory(), Snippets: idx,
	}
	pkt, err := pipe.Search(ctx, retrieval.Request{
		Credentials: ports.Credentials{BearerToken: "local:org_del_1:alice:employee"},
		OrgID:       org, Query: "billing", Purpose: "support", Limit: 10, Action: "context.search",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt.Citations) == 0 {
		t.Fatal("expected pre-delete hit")
	}

	del := &deletion.Service{
		Ledger: store, Evidence: memory.NewEvidence(), Index: idx, Authz: authz,
		Audit: audit.NewMemory(), Changes: store, SignKey: []byte("test-signing-key"),
	}
	manifest, err := del.Run(ctx, deletion.Request{
		OrgID: org, ResourceID: rec.ResourceID,
		Principal: ports.Principal{ID: "local|alice", Kind: ports.PrincipalKindUser, OrgID: org, Subject: "alice"},
		Reason:    "user_request",
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "completed" && manifest.Status != "blocked" {
		t.Fatalf("unexpected status %s", manifest.Status)
	}
	if !deletion.VerifySignature([]byte("test-signing-key"), manifest) {
		t.Fatal("manifest signature invalid")
	}

	// Even if index still had the doc, ledger tombstone must dominate.
	_ = idx.Upsert(ctx, []ports.IndexDocument{{
		ResourceID: rec.ResourceID, RevisionID: "rev1", OrgID: org,
		Text: "delete-me secret billing", Labels: []string{"purpose:support"},
		Attributes: map[string]string{"classification": "internal", "purpose_allowlist": "support"},
	}})

	pkt2, err := pipe.Search(ctx, retrieval.Request{
		Credentials: ports.Credentials{BearerToken: "local:org_del_1:alice:employee"},
		OrgID:       org, Query: "billing", Purpose: "support", Limit: 10, Action: "context.search",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt2.Citations) != 0 {
		t.Fatalf("tombstone must dominate search; got %d citations", len(pkt2.Citations))
	}
}
