package retrieval_test

import (
	"context"
	"testing"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/adapters/openfga"
	"github.com/xsama/context-fabric/internal/audit"
	"github.com/xsama/context-fabric/internal/authn"
	"github.com/xsama/context-fabric/internal/policy"
	"github.com/xsama/context-fabric/internal/ports"
	"github.com/xsama/context-fabric/internal/retrieval"
)

func TestTagCannotWidenAccess(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	idx := memory.NewIndex()
	authz := openfga.NewMemory()
	org := "org_acme_0001"
	_ = store.CreateOrganization(ctx, ports.Organization{ID: org, Name: "Acme"})

	// Restricted resource; free tags claim "classification:public" but system classification is restricted.
	rec := ports.Record{
		ResourceID:     "res_note_restricted_009",
		OrgID:          org,
		Kind:           "document",
		Title:          "secret note",
		Classification: "restricted",
		Labels:         []string{"classification:public", "visibility:public"},
		CurrentRevID:   "rev1",
		State:          "INDEXED",
	}
	_ = store.WithOrgTx(ctx, org, func(ctx context.Context, tx ports.Tx) error {
		return store.UpsertRecord(ctx, tx, rec)
	})
	_ = idx.Upsert(ctx, []ports.IndexDocument{{
		ResourceID: rec.ResourceID,
		RevisionID: "rev1",
		OrgID:      org,
		Text:       "billing secret",
		Labels:     []string{"classification:public", "visibility:public", "purpose:support"},
		Attributes: map[string]string{
			"classification":    "restricted", // system field
			"purpose_allowlist": "support",
		},
	}})

	// Bob is org member but has NO can_read on the restricted resource.
	authz.AddOrgMember(org, "bob")
	// Do not Grant can_read for bob.

	pipe := &retrieval.Pipeline{
		Identity: authn.NewLocal(),
		Authz:    authz,
		Policy:   policy.New(),
		Ledger:   store,
		Index:    idx,
		Audit:    audit.NewMemory(),
		Snippets: idx,
	}

	pkt, err := pipe.Search(ctx, retrieval.Request{
		Credentials: ports.Credentials{BearerToken: "local:org_acme_0001:bob:employee"},
		OrgID:       org,
		Query:       "billing",
		Purpose:     "support",
		Limit:       10,
		Filters:     map[string]string{"include_tags": "classification:public"},
		Consistency: ports.ConsistencyFullyConsistent,
		Action:      "context.search",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(pkt.Citations) != 0 {
		t.Fatalf("tags must not widen access; got %d citations", len(pkt.Citations))
	}
	if pkt.AuditID == "" {
		t.Fatal("expected audit_id on deny/empty packet")
	}
}

func TestSearchReturnsCitationsAndAuditID(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	idx := memory.NewIndex()
	authz := openfga.NewMemory()
	org := "org_acme_0001"
	_ = store.CreateOrganization(ctx, ports.Organization{ID: org, Name: "Acme"})

	rec := ports.Record{
		ResourceID:     "res_ok_1",
		OrgID:          org,
		Kind:           "document",
		Title:          "billing guide",
		Classification: "internal",
		CurrentRevID:   "rev1",
		State:          "INDEXED",
	}
	_ = store.WithOrgTx(ctx, org, func(ctx context.Context, tx ports.Tx) error {
		return store.UpsertRecord(ctx, tx, rec)
	})
	_ = idx.Upsert(ctx, []ports.IndexDocument{{
		ResourceID: rec.ResourceID,
		RevisionID: "rev1",
		OrgID:      org,
		Text:       "how to handle billing questions",
		Labels:     []string{"topic:billing", "purpose:support"},
		Attributes: map[string]string{"classification": "internal", "purpose_allowlist": "support"},
	}})
	authz.AddOrgMember(org, "alice")
	authz.Grant("resource:res_ok_1", "can_read", "user:alice")

	pipe := &retrieval.Pipeline{
		Identity: authn.NewLocal(),
		Authz:    authz,
		Policy:   policy.New(),
		Ledger:   store,
		Index:    idx,
		Audit:    audit.NewMemory(),
		Snippets: idx,
	}

	pkt, err := pipe.Search(ctx, retrieval.Request{
		Credentials: ports.Credentials{BearerToken: "local:org_acme_0001:alice:employee"},
		OrgID:       org,
		Query:       "billing",
		Purpose:     "support",
		Limit:       10,
		Action:      "context.search",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(pkt.Citations) == 0 {
		t.Fatal("expected citations")
	}
	if pkt.Citations[0].ResourceID != "res_ok_1" {
		t.Fatalf("citation resource %s", pkt.Citations[0].ResourceID)
	}
	if pkt.AuditID == "" {
		t.Fatal("expected audit_id")
	}
}

func TestGraphHidesDeniedNeighbors(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	idx := memory.NewIndex()
	authz := openfga.NewMemory()
	org := "org_acme_0001"
	_ = store.CreateOrganization(ctx, ports.Organization{ID: org, Name: "Acme"})

	caseRec := ports.Record{ResourceID: "res_case", OrgID: org, Kind: "case", Title: "Case", Classification: "internal", CurrentRevID: "r1", State: "INDEXED"}
	noteOK := ports.Record{ResourceID: "res_note_ok", OrgID: org, Kind: "note", Title: "Visible note", Classification: "internal", CurrentRevID: "r1", State: "INDEXED"}
	noteSecret := ports.Record{ResourceID: "res_note_secret", OrgID: org, Kind: "note", Title: "Secret note", Classification: "restricted", CurrentRevID: "r1", State: "INDEXED"}
	_ = store.WithOrgTx(ctx, org, func(ctx context.Context, tx ports.Tx) error {
		for _, rec := range []ports.Record{caseRec, noteOK, noteSecret} {
			if err := store.UpsertRecord(ctx, tx, rec); err != nil {
				return err
			}
		}
		if err := store.UpsertEdge(ctx, tx, ports.GraphEdge{
			EdgeID: "e1", OrgID: org, FromID: "res_note_ok", ToID: "res_case", Predicate: ports.EdgeParent,
		}); err != nil {
			return err
		}
		return store.UpsertEdge(ctx, tx, ports.GraphEdge{
			EdgeID: "e2", OrgID: org, FromID: "res_note_secret", ToID: "res_case", Predicate: ports.EdgeParent,
		})
	})

	authz.AddOrgMember(org, "alice")
	authz.Grant("resource:res_case", "can_read", "user:alice")
	authz.Grant("resource:res_note_ok", "can_read", "user:alice")
	// No grant for res_note_secret.

	pipe := &retrieval.Pipeline{
		Identity: authn.NewLocal(),
		Authz:    authz,
		Policy:   policy.New(),
		Ledger:   store,
		Index:    idx,
		Audit:    audit.NewMemory(),
	}

	pkt, err := pipe.Graph(ctx, retrieval.Request{
		Credentials: ports.Credentials{BearerToken: "local:org_acme_0001:alice:employee"},
		OrgID:       org,
		Purpose:     "support",
		ResourceID:  "res_case",
		Depth:       1,
		Action:      "context.graph",
	})
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	ids := map[string]bool{}
	for _, n := range pkt.Nodes {
		ids[n.ResourceID] = true
	}
	if !ids["res_case"] || !ids["res_note_ok"] {
		t.Fatalf("expected case + visible note, got %#v", ids)
	}
	if ids["res_note_secret"] {
		t.Fatal("secret neighbor must not appear")
	}
	for _, e := range pkt.Edges {
		if e.FromID == "res_note_secret" || e.ToID == "res_note_secret" {
			t.Fatalf("edge leaked denied node: %#v", e)
		}
	}
	if len(pkt.Edges) != 1 {
		t.Fatalf("expected 1 visible edge, got %d", len(pkt.Edges))
	}
}

func TestGraphHidesPlaceholders(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	authz := openfga.NewMemory()
	org := "org_acme_0001"
	_ = store.CreateOrganization(ctx, ports.Organization{ID: org, Name: "Acme"})

	seed := ports.Record{ResourceID: "res_seed", OrgID: org, Kind: "note", Title: "Seed", Classification: "internal", CurrentRevID: "r1", State: "INDEXED"}
	ph := ports.Record{ResourceID: "res_ph", OrgID: org, Kind: "resource", Title: "Stub", Classification: "internal", State: ports.LifecyclePlaceholder}
	_ = store.WithOrgTx(ctx, org, func(ctx context.Context, tx ports.Tx) error {
		_ = store.UpsertRecord(ctx, tx, seed)
		if _, err := store.InsertPlaceholder(ctx, tx, ph); err != nil {
			return err
		}
		return store.UpsertEdge(ctx, tx, ports.GraphEdge{
			EdgeID: "e-ph", OrgID: org, FromID: "res_seed", ToID: "res_ph", Predicate: ports.EdgeMentions, State: "ACTIVE",
		})
	})
	authz.AddOrgMember(org, "alice")
	authz.Grant("resource:res_seed", "reader", "user:alice")
	authz.Grant("resource:res_ph", "reader", "user:alice")

	pipe := &retrieval.Pipeline{
		Identity: authn.NewLocal(),
		Authz:    authz,
		Policy:   policy.New(),
		Ledger:   store,
		Index:    memory.NewIndex(),
		Audit:    audit.NewMemory(),
	}
	pkt, err := pipe.Graph(ctx, retrieval.Request{
		Credentials: ports.Credentials{BearerToken: "local:org_acme_0001:alice:employee"},
		OrgID:       org,
		Purpose:     "support",
		ResourceID:  "res_seed",
		Depth:       1,
		Action:      "context.graph",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range pkt.Nodes {
		if n.ResourceID == "res_ph" {
			t.Fatal("placeholder must not be retrievable")
		}
	}
	for _, e := range pkt.Edges {
		if e.ToID == "res_ph" || e.FromID == "res_ph" {
			t.Fatalf("placeholder edge must not leak: %#v", e)
		}
	}
}

func TestGraphParentInheritance(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	authz := openfga.NewMemory()
	org := "org_acme_0001"
	_ = store.CreateOrganization(ctx, ports.Organization{ID: org, Name: "Acme"})

	parent := ports.Record{ResourceID: "res_parent", OrgID: org, Kind: "case", Title: "Parent", Classification: "internal", CurrentRevID: "r1", State: "INDEXED"}
	child := ports.Record{ResourceID: "res_child", OrgID: org, Kind: "note", Title: "Child", Classification: "internal", CurrentRevID: "r1", State: "INDEXED"}
	_ = store.WithOrgTx(ctx, org, func(ctx context.Context, tx ports.Tx) error {
		_ = store.UpsertRecord(ctx, tx, parent)
		_ = store.UpsertRecord(ctx, tx, child)
		return store.UpsertEdge(ctx, tx, ports.GraphEdge{
			EdgeID: "e_parent", OrgID: org, FromID: "res_child", ToID: "res_parent", Predicate: ports.EdgeParent,
		})
	})
	authz.AddOrgMember(org, "alice")
	authz.Grant("resource:res_parent", "can_read", "user:alice")
	authz.Grant("resource:res_child", "parent", "resource:res_parent")

	pipe := &retrieval.Pipeline{
		Identity: authn.NewLocal(),
		Authz:    authz,
		Policy:   policy.New(),
		Ledger:   store,
		Index:    memory.NewIndex(),
		Audit:    audit.NewMemory(),
	}
	pkt, err := pipe.Graph(ctx, retrieval.Request{
		Credentials: ports.Credentials{BearerToken: "local:org_acme_0001:alice:employee"},
		OrgID:       org,
		Purpose:     "support",
		ResourceID:  "res_parent",
		Depth:       1,
		Action:      "context.graph",
	})
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	found := false
	for _, n := range pkt.Nodes {
		if n.ResourceID == "res_child" {
			found = true
		}
	}
	if !found {
		t.Fatal("child should inherit can_read from parent AuthZ tuple")
	}
}

type searchOnlyIdentity struct{}

func (searchOnlyIdentity) Discover(context.Context) (ports.OIDCMetadata, error) {
	return ports.OIDCMetadata{}, nil
}

func (searchOnlyIdentity) Authenticate(_ context.Context, _ ports.Credentials) (ports.Principal, error) {
	return ports.Principal{
		ID: "local|searcher", Kind: ports.PrincipalKindUser, OrgID: "org_scope",
		Subject: "searcher", Scopes: []string{"context:search"},
	}, nil
}

func TestGraphRequiresReadScope(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	authz := openfga.NewMemory()
	org := "org_scope"
	_ = store.CreateOrganization(ctx, ports.Organization{ID: org, Name: "Scope"})
	_ = store.WithOrgTx(ctx, org, func(ctx context.Context, tx ports.Tx) error {
		return store.UpsertRecord(ctx, tx, ports.Record{
			ResourceID: "seed", OrgID: org, Kind: "note", Title: "s",
			Classification: "internal", CurrentRevID: "r1", State: "INDEXED",
		})
	})
	authz.AddOrgMember(org, "searcher")
	authz.Grant("resource:seed", "reader", "user:searcher")

	pipe := &retrieval.Pipeline{
		Identity: searchOnlyIdentity{},
		Authz:    authz,
		Policy:   policy.New(),
		Ledger:   store,
		Index:    memory.NewIndex(),
		Audit:    audit.NewMemory(),
	}
	_, err := pipe.Graph(ctx, retrieval.Request{
		Credentials: ports.Credentials{BearerToken: "unused"},
		OrgID:       org,
		Purpose:     "support",
		ResourceID:  "seed",
		Action:      "context.graph",
	})
	if err == nil {
		t.Fatal("context:search alone must not authorize context.graph")
	}
}
