package ingest_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/adapters/openfga"
	"github.com/xsama/context-fabric/internal/ingest"
	"github.com/xsama/context-fabric/internal/mapping"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

func setup(t *testing.T) (*ingest.IntakeService, *memory.Store, ports.SourceRegistration) {
	t.Helper()
	ledger := memory.NewStore()
	ev := memory.NewEvidence()
	_ = ledger.CreateOrganization(context.Background(), ports.Organization{ID: "org1", Name: "Org"})
	src := ports.SourceRegistration{
		SourceID:              "src1",
		OrgID:                 "org1",
		System:                "chatwoot",
		Enabled:               true,
		TrustCeiling:          "trusted_internal",
		TrustTier:            "trusted_internal",
		AuthorityCeiling:      "source_of_truth",
		ClassificationCeiling: "confidential",
		SigningSecret:         "super-secret",
		ReplayWindowSeconds:   300,
		AllowedRecordTypes:    []string{"message", "event", "observation"},
		AllowedVisibilityRefs: []string{"case:1#viewer"},
	}
	_ = ledger.WithOrgTx(context.Background(), "org1", func(ctx context.Context, tx ports.Tx) error {
		return ledger.UpsertSource(ctx, tx, src)
	})
	svc := &ingest.IntakeService{Ledger: ledger, Evidence: ev, Now: func() time.Time {
		return time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	}}
	return svc, ledger, src
}

func baseSpec() mapping.Spec {
	return mapping.Spec{
		OrganizationID: "org1",
		SourceID:       "src1",
		Mappings: mapping.FieldMappings{
			SourceExternalID: &mapping.Expr{Expr: "$.data.source_external_id"},
			SourceRevision:   &mapping.Expr{Expr: "$.data.source_revision"},
			ContextSpaceID:   &mapping.Expr{Expr: `"cs1"`},
			ResourceID:       &mapping.Expr{Expr: "$.data.resource_id"},
			ResourceType:     &mapping.Expr{Expr: "$.data.resource_type"},
			ContentLocator:   &mapping.Expr{Expr: `"ev://1"`},
			Classification:   &mapping.Expr{Expr: "$.data.classification"},
			Trust:            &mapping.Expr{Expr: "$.data.trust"},
			SourceAuthority:  &mapping.Expr{Expr: "$.data.source_authority"},
			VisibilityRef:    &mapping.Expr{Expr: "$.data.visibility_ref"},
			Title:            &mapping.Expr{Expr: "$.data.title"},
			Timestamps: mapping.TimestampMappings{
				OccurredAt: &mapping.Expr{Expr: `"2024-06-01T12:00:00Z"`},
				ObservedAt: &mapping.Expr{Expr: `"2024-06-01T12:00:00Z"`},
			},
		},
	}
}

func signedReq(t *testing.T, secret string, payload map[string]any, idem string) ingest.Request {
	t.Helper()
	body, _ := json.Marshal(payload)
	ts := strconv.FormatInt(time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC).Unix(), 10)
	return ingest.Request{
		OrgID:          "org1",
		SourceID:       "src1",
		Body:           body,
		Signature:      ingest.SignHMAC(secret, body),
		Timestamp:      ts,
		IdempotencyKey: idem,
		Mapping:        baseSpec(),
		Payload:        payload,
		Event:          ports.IntakeEvent{EventID: "evt-" + idem, IdempotencyKey: idem},
	}
}

func TestDuplicateIntakeIdempotent(t *testing.T) {
	svc, _, src := setup(t)
	payload := map[string]any{
		"data": map[string]any{
			"source_external_id": "ext-1",
			"source_revision":    "r1",
			"resource_id":        "res-1",
			"resource_type":      "message",
			"classification":     "internal",
			"trust":              "trusted_internal",
			"source_authority":   "corroborating",
			"visibility_ref":     "case:1#viewer",
			"title":              "Hello",
		},
	}
	req := signedReq(t, src.SigningSecret, payload, "idem-1")
	r1, err := svc.Intake(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Duplicate || r1.RevisionID == "" {
		t.Fatalf("first: %+v", r1)
	}
	r2, err := svc.Intake(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !r2.Duplicate || !r2.IdempotentReplay {
		t.Fatalf("expected duplicate replay: %+v", r2)
	}
	if r2.EventID != r1.EventID || r2.RevisionID != r1.RevisionID {
		t.Fatalf("replay mismatch: %+v vs %+v", r1, r2)
	}
}

func TestIntakeEmitsParentEdgeFromVisibilityRef(t *testing.T) {
	svc, ledger, src := setup(t)
	authz := openfga.NewMemory()
	svc.Authz = authz
	payload := map[string]any{
		"data": map[string]any{
			"source_external_id": "ext-edge-1",
			"source_revision":    "r1",
			"resource_id":        "res-msg-1",
			"resource_type":      "message",
			"classification":     "internal",
			"trust":              "trusted_internal",
			"source_authority":   "corroborating",
			"visibility_ref":     "case:1#viewer",
			"title":              "Hello",
		},
	}
	res, err := svc.Intake(context.Background(), signedReq(t, src.SigningSecret, payload, "idem-edge-1"))
	if err != nil {
		t.Fatal(err)
	}
	if res.EdgeCount != 1 {
		t.Fatalf("expected 1 edge, got %d", res.EdgeCount)
	}
	edges, err := ledger.ListEdges(context.Background(), "org1", ports.EdgeListOptions{
		ResourceIDs: []string{"res-msg-1"},
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 || edges[0].Predicate != ports.EdgeParent || edges[0].FromID != "res-msg-1" {
		t.Fatalf("unexpected edges: %#v", edges)
	}
	parentID := edges[0].ToID
	if _, err := ledger.GetRecord(context.Background(), "org1", parentID); err != nil {
		t.Fatalf("expected ensured parent stub: %v", err)
	}
	// OpenFGA parent tuple synced for inheritance.
	dec, err := authz.Check(context.Background(), ports.AuthzCheck{
		Principal:  ports.Principal{Kind: ports.PrincipalKindUser, Subject: "alice", OrgID: "org1"},
		Action:     "can_read",
		ResourceID: "res-msg-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Without grants on parent, child alone has no can_read — tuple exists but no reader yet.
	_ = dec
	authz.Grant("resource:"+parentID, "can_read", "user:alice")
	dec, err = authz.Check(context.Background(), ports.AuthzCheck{
		Principal:  ports.Principal{Kind: ports.PrincipalKindUser, Subject: "alice", OrgID: "org1"},
		Action:     "can_read",
		ResourceID: "res-msg-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dec.Allowed {
		t.Fatal("child should inherit can_read via synced parent tuple")
	}
}

func TestIntakeExplicitEdgesAndParentResourceID(t *testing.T) {
	svc, ledger, src := setup(t)
	req := signedReq(t, src.SigningSecret, map[string]any{
		"data": map[string]any{
			"source_external_id":  "ext-edge-2",
			"source_revision":     "r1",
			"resource_id":         "res-msg-2",
			"resource_type":       "message",
			"classification":      "internal",
			"trust":               "trusted_internal",
			"source_authority":    "corroborating",
			"visibility_ref":      "case:1#viewer",
			"title":               "Hello",
			"parent_resource_id":  "res-case-explicit",
			"mentioned_person_id": "res-person-9",
		},
	}, "idem-edge-2")
	req.Mapping.Mappings.ParentResourceID = &mapping.Expr{Expr: "$.data.parent_resource_id"}
	req.Mapping.Edges = []mapping.EdgeMapping{{
		Predicate:       &mapping.Expr{Expr: `"mentions"`},
		To:              &mapping.Expr{Expr: "$.data.mentioned_person_id"},
		SyncAuthzParent: boolPtr(false),
	}}
	res, err := svc.Intake(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.EdgeCount != 2 {
		t.Fatalf("expected parent+mentions (visibility auto suppressed), got %d", res.EdgeCount)
	}
	edges, err := ledger.ListEdges(context.Background(), "org1", ports.EdgeListOptions{
		ResourceIDs: []string{"res-msg-2"},
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	preds := map[string]string{}
	for _, e := range edges {
		preds[e.Predicate] = e.ToID
	}
	if preds[ports.EdgeParent] != "res-case-explicit" {
		t.Fatalf("parent edge: %#v", preds)
	}
	if preds[ports.EdgeMentions] != "res-person-9" {
		t.Fatalf("mentions edge: %#v", preds)
	}
}

func boolPtr(v bool) *bool { return &v }

func TestMappingCannotBroadenTrust(t *testing.T) {
	svc, _, src := setup(t)
	payload := map[string]any{
		"data": map[string]any{
			"source_external_id": "ext-2",
			"source_revision":    "r1",
			"resource_id":        "res-2",
			"resource_type":      "message",
			"classification":     "internal",
			"trust":              "trusted_system",
			"source_authority":   "corroborating",
			"visibility_ref":     "case:1#viewer",
			"title":              "Nope",
		},
	}
	_, err := svc.Intake(context.Background(), signedReq(t, src.SigningSecret, payload, "idem-2"))
	if err == nil {
		t.Fatal("expected trust ceiling error")
	}
}

func TestHMACFail(t *testing.T) {
	svc, _, src := setup(t)
	payload := map[string]any{"data": map[string]any{
		"source_external_id": "ext-3", "source_revision": "r1", "resource_id": "res-3",
		"resource_type": "message", "classification": "internal", "trust": "trusted_internal",
		"source_authority": "corroborating", "visibility_ref": "case:1#viewer", "title": "x",
	}}
	req := signedReq(t, src.SigningSecret, payload, "idem-3")
	req.Signature = "deadbeef"
	_, err := svc.Intake(context.Background(), req)
	if err == nil {
		t.Fatal("expected hmac failure")
	}
	ae, ok := platform.AsAPIError(err)
	if !ok || ae.HTTPStatus != 401 {
		t.Fatalf("want 401, got %v", err)
	}
}

func TestGeneratedObservationCannotOverwrite(t *testing.T) {
	svc, ledger, src := setup(t)
	// Seed source_of_truth record.
	_ = ledger.WithOrgTx(context.Background(), "org1", func(ctx context.Context, tx ports.Tx) error {
		return ledger.UpsertRecord(ctx, tx, ports.Record{
			ResourceID: "res-sot", OrgID: "org1", Kind: "message", Title: "Canonical",
			Classification: "internal", State: "ACCEPTED",
			Attributes: map[string]string{"authority": "source_of_truth", "trust": "trusted_internal"},
			CreatedAt:  time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		})
	})
	payload := map[string]any{"data": map[string]any{
		"source_external_id": "ext-4", "source_revision": "obs-1", "resource_id": "res-sot",
		"resource_type": "observation", "classification": "confidential", "trust": "generated",
		"source_authority": "user_claim", "visibility_ref": "case:1#viewer", "title": "Overwrite attempt",
	}}
	_, err := svc.Intake(context.Background(), signedReq(t, src.SigningSecret, payload, "idem-4"))
	if err == nil {
		t.Fatal("expected overwrite rejection")
	}
	if !platform.IsAPIError(err) {
		t.Fatalf("want api error, got %v", err)
	}
}
