package app_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/adapters/openfga"
	"github.com/xsama/context-fabric/internal/application"
	"github.com/xsama/context-fabric/internal/audit"
	"github.com/xsama/context-fabric/internal/authn"
	"github.com/xsama/context-fabric/internal/changes"
	"github.com/xsama/context-fabric/internal/ingest"
	"github.com/xsama/context-fabric/internal/mapping"
	"github.com/xsama/context-fabric/internal/ports"
	"github.com/xsama/context-fabric/internal/policy"
	"github.com/xsama/context-fabric/internal/retrieval"
)

func newP1Svc(t *testing.T) (*app.ApplicationService, *memory.Store, string, ports.Credentials) {
	t.Helper()
	org := "org_p1_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	store := memory.NewStore()
	idx := memory.NewIndex()
	ev := memory.NewEvidence()
	authz := openfga.NewMemory()
	identity := authn.NewLocal()
	auditLog := audit.NewMemory()
	_ = store.CreateOrganization(context.Background(), ports.Organization{ID: org, Name: "P1"})
	authz.AddOrgMember(org, "alice")
	authz.AddOrgMember(org, "user:alice")

	pipe := &retrieval.Pipeline{
		Identity: identity, Authz: authz, Policy: policy.New(),
		Ledger: store, Index: idx, Audit: auditLog, Snippets: idx,
	}
	svc := &app.ApplicationService{
		Identity: identity, Authz: authz, Policy: policy.New(),
		Ledger: store, Evidence: ev, Index: idx, Audit: auditLog,
		Retrieve: pipe, Ingest: &ingest.IntakeService{Ledger: store, Evidence: ev},
		Changes: store, Extras: store, ChangeFeed: changes.New(store, []byte("p1-wh")),
		Mappings: store,
		MappingBySource: func(ctx context.Context, orgID, sourceID string) (mapping.Spec, error) {
			return store.GetMappingSpec(ctx, orgID, sourceID)
		},
		Ready: func() bool { return true },
	}
	creds := ports.Credentials{BearerToken: "local:" + org + ":alice:owner"}
	return svc, store, org, creds
}

func TestIntakeBatch(t *testing.T) {
	ctx := context.Background()
	svc, _, org, creds := newP1Svc(t)
	src, err := svc.RegisterSource(ctx, creds, org, ports.SourceRegistration{
		SourceID: "src_batch", System: "synthetic", Enabled: true,
		TrustCeiling: "trusted_internal", AuthorityCeiling: "source_of_truth",
		ClassificationCeiling: "internal", SigningSecret: "batch-secret",
		ReplayWindowSeconds: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Now().UTC()
	mk := func(id, external string) app.IntakeRequest {
		payload := map[string]any{
			"id": id, "source": src.SourceID,
			"data": map[string]any{
				"source_external_id": external, "source_revision": "1",
				"resource_id": "res_" + external, "resource_type": "event",
				"classification": "internal", "trust": "trusted_internal",
				"source_authority": "corroborating", "title": external,
			},
		}
		body, _ := json.Marshal(payload)
		return app.IntakeRequest{
			Event: ports.IntakeEvent{EventID: id, IdempotencyKey: "idem-" + id, SourceSystem: src.SourceID},
			Body: body, Signature: ingest.SignHMAC(src.SigningSecret, body),
			Timestamp: strconv.FormatInt(fixed.Unix(), 10),
			SourceID: src.SourceID, Payload: payload,
		}
	}
	out, err := svc.IntakeBatch(ctx, creds, org, []app.IntakeRequest{
		mk("evt_b1", "ext1"), mk("evt_b2", "ext2"),
	})
	if err != nil {
		t.Fatal(err)
	}
	results, _ := out["results"].([]map[string]any)
	if results == nil {
		if raw, ok := out["results"].([]any); ok {
			if len(raw) != 2 {
				t.Fatalf("want 2 results, got %d: %#v", len(raw), out)
			}
		} else {
			t.Fatalf("missing results: %#v", out)
		}
	} else if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	if out["accepted_count"] != 2 {
		t.Fatalf("accepted_count=%v", out["accepted_count"])
	}
}

func TestRegisterSourceRequireOrgDeny(t *testing.T) {
	ctx := context.Background()
	svc, _, org, _ := newP1Svc(t)
	otherCreds := ports.Credentials{BearerToken: "local:org_other:bob:owner"}
	_, err := svc.RegisterSource(ctx, otherCreds, org, ports.SourceRegistration{
		SourceID: "src_x", System: "synthetic", Enabled: true, SigningSecret: "s",
	})
	if err == nil {
		t.Fatal("expected cross-org deny")
	}
}

func TestRotateSource(t *testing.T) {
	ctx := context.Background()
	svc, store, org, creds := newP1Svc(t)
	src, err := svc.RegisterSource(ctx, creds, org, ports.SourceRegistration{
		SourceID: "src_rot", System: "synthetic", Enabled: true, SigningSecret: "old-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.RotateSourceSecret(ctx, creds, org, src.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	secret, _ := out["secret_once"].(string)
	keyID, _ := out["key_id"].(string)
	if secret == "" || secret == "old-secret" || keyID == "" {
		t.Fatalf("bad rotate result: %#v", out)
	}
	got, err := store.GetSource(ctx, org, src.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attributes["signing_secret"] != secret {
		t.Fatalf("persisted secret mismatch")
	}
	// GetSource must not leak secret.
	pub, err := svc.GetSource(ctx, creds, org, src.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if pub.SigningSecret != "" || (pub.Attributes != nil && pub.Attributes["signing_secret"] != "") {
		t.Fatal("GetSource leaked signing secret")
	}
}

func TestOpsLagDoesNotClaimOutbox(t *testing.T) {
	ctx := context.Background()
	svc, store, org, creds := newP1Svc(t)
	_ = store.WithOrgTx(ctx, org, func(ctx context.Context, tx ports.Tx) error {
		return store.EnqueueOutbox(ctx, tx, ports.OutboxEntry{
			ID: "ob1", OrgID: org, Subject: "test", Payload: []byte(`{}`), CreatedAt: time.Now().UTC(),
		})
	})
	before, err := store.ListOutboxPending(ctx, org, 10)
	if err != nil || len(before) != 1 {
		t.Fatalf("setup pending=%d err=%v", len(before), err)
	}
	out, err := svc.OpsLag(ctx, creds, org)
	if err != nil {
		t.Fatal(err)
	}
	if out["outbox_pending"] != 1 {
		t.Fatalf("outbox_pending=%v", out["outbox_pending"])
	}
	// Claiming would lease the row; ListOutboxPending must still see it, and
	// ClaimOutbox should still be able to claim (not already leased by OpsLag).
	claimed, err := store.ClaimOutbox(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 {
		t.Fatalf("OpsLag must not lease outbox rows; ClaimOutbox got %d", len(claimed))
	}
}

func TestRequestAccessPersistsJustification(t *testing.T) {
	ctx := context.Background()
	svc, store, org, creds := newP1Svc(t)
	out, err := svc.RequestAccess(ctx, creds, org, "res1", "support", "need billing docs")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := out["request_id"].(string)
	if id == "" {
		t.Fatal("empty request_id")
	}
	if out["justification"] != "need billing docs" {
		t.Fatalf("justification discarded: %#v", out)
	}
	req, err := store.GetAccessRequest(ctx, org, id)
	if err != nil {
		t.Fatal(err)
	}
	if req.Justification != "need billing docs" {
		t.Fatalf("stored justification=%q", req.Justification)
	}
}

func TestVerifySourceRequiresSecret(t *testing.T) {
	ctx := context.Background()
	svc, _, org, creds := newP1Svc(t)
	_, err := svc.RegisterSource(ctx, creds, org, ports.SourceRegistration{
		SourceID: "src_nosec", System: "synthetic", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.VerifySource(ctx, creds, org, "src_nosec")
	if err != nil {
		t.Fatal(err)
	}
	if out["status"] != "failed" {
		t.Fatalf("expected failed, got %#v", out)
	}
	_, err = svc.RegisterSource(ctx, creds, org, ports.SourceRegistration{
		SourceID: "src_ok", System: "synthetic", Enabled: true, SigningSecret: "ok-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err = svc.VerifySource(ctx, creds, org, "src_ok")
	if err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" {
		t.Fatalf("expected ok, got %#v", out)
	}
}
