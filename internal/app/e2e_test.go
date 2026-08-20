package app_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/adapters/openfga"
	"github.com/xsama/context-fabric/internal/app"
	"github.com/xsama/context-fabric/internal/audit"
	"github.com/xsama/context-fabric/internal/authn"
	"github.com/xsama/context-fabric/internal/changes"
	"github.com/xsama/context-fabric/internal/deletion"
	"github.com/xsama/context-fabric/internal/export"
	"github.com/xsama/context-fabric/internal/ingest"
	"github.com/xsama/context-fabric/internal/mapping"
	"github.com/xsama/context-fabric/internal/ports"
	"github.com/xsama/context-fabric/internal/policy"
	"github.com/xsama/context-fabric/internal/retrieval"
)

// TestE2EMemoryHappyPathAndRevoke boots an in-memory ApplicationService and walks
// register → intake → search → brief → change → delete (tombstone) → empty search → export.
func TestE2EMemoryHappyPathAndRevoke(t *testing.T) {
	ctx := context.Background()
	org := "org_e2e_mem_1"
	token := "local:" + org + ":alice:owner"
	creds := ports.Credentials{BearerToken: token}
	scopes := []string{"context:search", "context:read", "context:ingest", "context:manage_policy"}

	store := memory.NewStore()
	idx := memory.NewIndex()
	ev := memory.NewEvidence()
	authz := openfga.NewMemory()
	identity := authn.NewLocal()
	auditLog := audit.NewMemory()
	changeFeed := changes.New(store, []byte("e2e-webhook-secret"))
	fixedNow := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	if err := store.CreateOrganization(ctx, ports.Organization{ID: org, Name: "E2E Memory"}); err != nil {
		t.Fatal(err)
	}
	authz.AddOrgMember(org, "alice")
	authz.AddOrgMember(org, "user:alice")

	pipe := &retrieval.Pipeline{
		Identity: identity,
		Authz:    authz,
		Policy:   policy.New(),
		Ledger:   store,
		Index:    idx,
		Audit:    auditLog,
		Snippets: idx,
	}
	intakeSvc := &ingest.IntakeService{
		Ledger:   store,
		Evidence: ev,
		Now:      func() time.Time { return fixedNow },
	}
	delSvc := &deletion.Service{
		Ledger: store, Evidence: ev, Index: idx, Authz: authz,
		Audit: auditLog, Changes: store,
	}
	exportSvc := &export.Service{Ledger: store, Now: func() time.Time { return fixedNow }}

	svc := &app.ApplicationService{
		Identity:   identity,
		Authz:      authz,
		Policy:     policy.New(),
		Ledger:     store,
		Evidence:   ev,
		Index:      idx,
		Audit:      auditLog,
		Retrieve:   pipe,
		Ingest:     intakeSvc,
		Changes:    store,
		Extras:     store,
		Deletion:   delSvc,
		Export:     exportSvc,
		ChangeFeed: changeFeed,
		ExportJobs: store,
		Ready:      func() bool { return true },
		Build: app.VersionInfo{
			ProductVersion: "0.0.0-e2e", APIVersion: "v1",
			PacketVersion: "v1", ExportFormatVersion: export.FormatVersion,
		},
	}

	src, err := svc.RegisterSource(ctx, creds, org, ports.SourceRegistration{
		SourceID:              "src_e2e",
		System:                "synthetic",
		Enabled:               true,
		TrustCeiling:          "trusted_internal",
		TrustTier:            "trusted_internal",
		AuthorityCeiling:      "source_of_truth",
		ClassificationCeiling: "confidential",
		SigningSecret:         "e2e-signing-secret",
		ReplayWindowSeconds:   300,
		AllowedRecordTypes:    []string{"message", "event", "observation"},
		AllowedVisibilityRefs: []string{"case:e2e#viewer"},
	})
	if err != nil {
		t.Fatalf("register source: %v", err)
	}

	spec := mapping.Spec{
		OrganizationID: org,
		SourceID:       src.SourceID,
		Mappings: mapping.FieldMappings{
			SourceExternalID: &mapping.Expr{Expr: "$.data.source_external_id"},
			SourceRevision:   &mapping.Expr{Expr: "$.data.source_revision"},
			ContextSpaceID:   &mapping.Expr{Expr: `"cs_e2e"`},
			ResourceID:       &mapping.Expr{Expr: "$.data.resource_id"},
			ResourceType:     &mapping.Expr{Expr: "$.data.resource_type"},
			ContentLocator:   &mapping.Expr{Expr: `"ev://e2e"`},
			Classification:   &mapping.Expr{Expr: "$.data.classification"},
			Trust:            &mapping.Expr{Expr: "$.data.trust"},
			SourceAuthority:  &mapping.Expr{Expr: "$.data.source_authority"},
			VisibilityRef:    &mapping.Expr{Expr: "$.data.visibility_ref"},
			Title:            &mapping.Expr{Expr: "$.data.title"},
			Timestamps: mapping.TimestampMappings{
				OccurredAt: &mapping.Expr{Expr: `"2026-08-20T12:00:00Z"`},
				ObservedAt: &mapping.Expr{Expr: `"2026-08-20T12:00:00Z"`},
			},
		},
	}

	payload := map[string]any{
		"data": map[string]any{
			"source_external_id": "ext-e2e-1",
			"source_revision":    "r1",
			"resource_id":        "res_e2e_doc",
			"resource_type":      "message",
			"classification":     "internal",
			"trust":              "trusted_internal",
			"source_authority":   "corroborating",
			"visibility_ref":     "case:e2e#viewer",
			"title":              "e2e billing playbook",
		},
	}
	body, _ := json.Marshal(payload)
	ts := strconv.FormatInt(fixedNow.Unix(), 10)
	intakeOut, err := svc.Intake(ctx, creds, org, app.IntakeRequest{
		Event:          ports.IntakeEvent{EventID: "evt_e2e_1", IdempotencyKey: "idem-e2e-1"},
		Body:           body,
		Signature:      ingest.SignHMAC(src.SigningSecret, body),
		Timestamp:      ts,
		IdempotencyKey: "idem-e2e-1",
		SourceID:       src.SourceID,
		Payload:        payload,
		Mapping:        spec,
	})
	if err != nil {
		t.Fatalf("intake: %v", err)
	}
	resourceID, _ := intakeOut["resource_id"].(string)
	if resourceID == "" {
		t.Fatalf("missing resource_id: %+v", intakeOut)
	}

	authz.Grant("resource:"+resourceID, "can_read", "user:alice")
	authz.Grant("resource:"+resourceID, "owner", "user:alice")
	authz.Grant("resource:"+resourceID, "can_delete", "user:alice")

	searchPkt, err := svc.Search(ctx, creds, org, scopes, app.SearchRequest{
		Query: "billing", Purpose: "support", MaxItems: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(searchPkt.Citations) == 0 {
		t.Fatal("expected search citations after intake")
	}

	briefPkt, err := svc.Brief(ctx, creds, org, scopes, "support", resourceID, "", 5)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	if len(briefPkt.Citations) == 0 {
		t.Fatal("expected brief citations")
	}

	changesOut, err := svc.ListChanges(ctx, creds, org, "", 20)
	if err != nil {
		t.Fatalf("list changes: %v", err)
	}
	if changeItemCount(changesOut["items"]) == 0 {
		t.Fatalf("expected change events after intake: %+v", changesOut)
	}

	delOut, err := svc.DeleteResource(ctx, creds, org, resourceID, "e2e_user_request")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if status, _ := delOut["status"].(string); status != "completed" && status != "blocked" {
		t.Fatalf("unexpected delete status: %+v", delOut)
	}

	searchAfter, err := svc.Search(ctx, creds, org, scopes, app.SearchRequest{
		Query: "billing", Purpose: "support", MaxItems: 10,
	})
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	if len(searchAfter.Citations) != 0 {
		t.Fatalf("tombstone must clear search; got %d citations", len(searchAfter.Citations))
	}

	exportOut, err := svc.StartExport(ctx, creds, org)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if status, _ := exportOut["status"].(string); status != "completed" {
		t.Fatalf("export not completed: %+v", exportOut)
	}
	manifest, ok := exportOut["manifest"].(export.Manifest)
	if !ok {
		t.Fatalf("missing export manifest: %+v", exportOut)
	}
	foundTomb := false
	for _, tomb := range manifest.Tombstones {
		if tomb.ResourceID == resourceID {
			foundTomb = true
			break
		}
	}
	if !foundTomb {
		// Records may still list TOMBSTONED state instead of dedicated slice.
		for _, rec := range manifest.Records {
			if rec.ResourceID == resourceID && rec.State == "TOMBSTONED" {
				foundTomb = true
				break
			}
		}
	}
	if !foundTomb {
		t.Fatalf("export should retain tombstone marker for %s", resourceID)
	}
}

func changeItemCount(v any) int {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return 0
	}
	return rv.Len()
}
