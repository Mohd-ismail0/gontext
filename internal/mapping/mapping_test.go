package mapping_test

import (
	"testing"

	"github.com/xsama/context-fabric/internal/mapping"
	"github.com/xsama/context-fabric/internal/platform"
)

func TestApplyMapsFields(t *testing.T) {
	spec := mapping.Spec{
		OrganizationID: "org1",
		SourceID:       "src1",
		Mappings: mapping.FieldMappings{
			SourceExternalID: &mapping.Expr{Expr: "$.conversation.id"},
			SourceRevision:   &mapping.Expr{Expr: "$.message.id"},
			ContextSpaceID:   &mapping.Expr{Expr: `"cs-support"`},
			ResourceID:       &mapping.Expr{Expr: "$.conversation.id", Transform: "uuid_v5"},
			ResourceType:     &mapping.Expr{Expr: `"message"`},
			ContentLocator:   &mapping.Expr{Expr: "$.evidence.uri"},
			Classification:   &mapping.Expr{Expr: `"internal"`},
			Trust:            &mapping.Expr{Expr: `"trusted_internal"`},
			SourceAuthority:  &mapping.Expr{Expr: `"corroborating"`},
			Title:            &mapping.Expr{Expr: "$.conversation.subject"},
			Timestamps: mapping.TimestampMappings{
				OccurredAt: &mapping.Expr{Expr: "$.message.created_at"},
				ObservedAt: &mapping.Expr{Expr: "$.message.created_at"},
			},
		},
	}
	payload := map[string]any{
		"conversation": map[string]any{"id": "42", "subject": "Billing"},
		"message":      map[string]any{"id": "99", "created_at": "2024-01-02T03:04:05Z"},
		"evidence":     map[string]any{"uri": "s3://bucket/k"},
	}
	ceilings := mapping.SourceCeilings{
		OrganizationID:        "org1",
		TrustCeiling:          "trusted_system",
		AuthorityCeiling:      "source_of_truth",
		ClassificationCeiling: "confidential",
		AllowedRecordTypes:    []string{"message", "event"},
	}
	got, err := mapping.Apply(spec, payload, ceilings, mapping.Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !got.DryRun {
		t.Fatal("expected dry run")
	}
	if got.SourceExternalID != "42" || got.Title != "Billing" {
		t.Fatalf("unexpected mapping: %+v", got)
	}
	if got.OrganizationID != "org1" {
		t.Fatalf("org: %s", got.OrganizationID)
	}
	if got.ResourceID == "" || got.ResourceID == "42" {
		t.Fatalf("expected uuid_v5 resource id, got %q", got.ResourceID)
	}
}

func TestApplyEmitsEdgesFromVisibilityAndParent(t *testing.T) {
	spec := mapping.Spec{
		OrganizationID: "org1",
		Mappings: mapping.FieldMappings{
			SourceExternalID: &mapping.Expr{Expr: `"ext"`},
			SourceRevision:   &mapping.Expr{Expr: `"r1"`},
			ContextSpaceID:   &mapping.Expr{Expr: `"cs"`},
			ResourceID:       &mapping.Expr{Expr: `"res-child"`},
			ResourceType:     &mapping.Expr{Expr: `"message"`},
			ContentLocator:   &mapping.Expr{Expr: `"ev://1"`},
			Classification:   &mapping.Expr{Expr: `"internal"`},
			Trust:            &mapping.Expr{Expr: `"trusted_internal"`},
			SourceAuthority:  &mapping.Expr{Expr: `"corroborating"`},
			VisibilityRef:    &mapping.Expr{Expr: `"case:SUP-1#viewer"`},
			ParentResourceID: &mapping.Expr{Expr: `"res-parent"`},
			Timestamps:       mapping.TimestampMappings{},
		},
		Edges: []mapping.EdgeMapping{{
			Predicate: &mapping.Expr{Expr: `"mentions"`},
			To:        &mapping.Expr{Expr: `"res-person"`},
		}},
	}
	got, err := mapping.Apply(spec, map[string]any{}, mapping.SourceCeilings{
		OrganizationID:        "org1",
		TrustCeiling:          "trusted_system",
		AuthorityCeiling:      "source_of_truth",
		ClassificationCeiling: "confidential",
	}, mapping.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Edges) != 2 {
		t.Fatalf("expected parent+mentions (visibility suppressed), got %#v", got.Edges)
	}
	preds := map[string]bool{}
	for _, e := range got.Edges {
		preds[e.Predicate] = true
		if e.FromID != "res-child" {
			t.Fatalf("from: %s", e.FromID)
		}
	}
	if !preds["parent"] || !preds["mentions"] {
		t.Fatalf("preds: %#v", preds)
	}
}

func TestRejectOrgMismatch(t *testing.T) {
	spec := mapping.Spec{
		OrganizationID: "org1",
		Mappings: mapping.FieldMappings{
			OrganizationID: &mapping.Expr{Expr: `"org-other"`},
			ResourceType:   &mapping.Expr{Expr: `"event"`},
			Timestamps:     mapping.TimestampMappings{},
		},
	}
	_, err := mapping.Apply(spec, map[string]any{}, mapping.SourceCeilings{OrganizationID: "org1"}, mapping.Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := platform.AsAPIError(err)
	if !ok || ae.HTTPStatus != 403 {
		t.Fatalf("want forbidden, got %v", err)
	}
}

func TestRejectTrustAboveCeiling(t *testing.T) {
	spec := mapping.Spec{
		OrganizationID: "org1",
		Mappings: mapping.FieldMappings{
			Trust:           &mapping.Expr{Expr: `"trusted_system"`},
			SourceAuthority: &mapping.Expr{Expr: `"corroborating"`},
			Classification:  &mapping.Expr{Expr: `"internal"`},
			ResourceType:    &mapping.Expr{Expr: `"event"`},
			Timestamps:      mapping.TimestampMappings{},
		},
	}
	_, err := mapping.Apply(spec, map[string]any{}, mapping.SourceCeilings{
		OrganizationID:        "org1",
		TrustCeiling:          "untrusted_external",
		AuthorityCeiling:      "source_of_truth",
		ClassificationCeiling: "restricted",
	}, mapping.Options{})
	if err == nil || !platform.IsAPIError(err) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestRejectAuthorityAboveCeiling(t *testing.T) {
	spec := mapping.Spec{
		OrganizationID: "org1",
		Mappings: mapping.FieldMappings{
			Trust:           &mapping.Expr{Expr: `"generated"`},
			SourceAuthority: &mapping.Expr{Expr: `"source_of_truth"`},
			Classification:  &mapping.Expr{Expr: `"internal"`},
			ResourceType:    &mapping.Expr{Expr: `"observation"`},
			Timestamps:      mapping.TimestampMappings{},
		},
	}
	_, err := mapping.Apply(spec, map[string]any{}, mapping.SourceCeilings{
		OrganizationID:        "org1",
		TrustCeiling:          "generated",
		AuthorityCeiling:      "user_claim",
		ClassificationCeiling: "internal",
		AllowedRecordTypes:    []string{"observation"},
	}, mapping.Options{})
	if err == nil {
		t.Fatal("expected authority ceiling rejection")
	}
}

func TestRejectVisibilityACLBroadening(t *testing.T) {
	spec := mapping.Spec{
		OrganizationID: "org1",
		Mappings: mapping.FieldMappings{
			VisibilityRef:   &mapping.Expr{Expr: `"case:OTHER#admin"`},
			Trust:           &mapping.Expr{Expr: `"trusted_internal"`},
			SourceAuthority: &mapping.Expr{Expr: `"corroborating"`},
			Classification:  &mapping.Expr{Expr: `"internal"`},
			ResourceType:    &mapping.Expr{Expr: `"event"`},
			Timestamps:      mapping.TimestampMappings{},
		},
	}
	_, err := mapping.Apply(spec, map[string]any{}, mapping.SourceCeilings{
		OrganizationID:        "org1",
		TrustCeiling:          "trusted_system",
		AuthorityCeiling:      "source_of_truth",
		ClassificationCeiling: "restricted",
		AllowedVisibilityRefs: []string{"case:SUP-1#viewer"},
	}, mapping.Options{})
	if err == nil {
		t.Fatal("expected ACL broadening rejection")
	}
}

func TestRejectClassificationAboveCeiling(t *testing.T) {
	spec := mapping.Spec{
		OrganizationID: "org1",
		Mappings: mapping.FieldMappings{
			Classification:  &mapping.Expr{Expr: `"restricted"`},
			Trust:           &mapping.Expr{Expr: `"trusted_internal"`},
			SourceAuthority: &mapping.Expr{Expr: `"corroborating"`},
			ResourceType:    &mapping.Expr{Expr: `"event"`},
			Timestamps:      mapping.TimestampMappings{},
		},
	}
	_, err := mapping.Apply(spec, map[string]any{}, mapping.SourceCeilings{
		OrganizationID:        "org1",
		TrustCeiling:          "trusted_system",
		AuthorityCeiling:      "source_of_truth",
		ClassificationCeiling: "internal",
	}, mapping.Options{})
	if err == nil {
		t.Fatal("expected classification ceiling rejection")
	}
}

func TestUnknownClassificationRejected(t *testing.T) {
	// Unknown enum previously ranked 0 and bypassed ceiling checks (0 is never > ceiling).
	spec := mapping.Spec{
		OrganizationID: "org1",
		Mappings: mapping.FieldMappings{
			Classification:  &mapping.Expr{Expr: `"top_secret"`},
			Trust:           &mapping.Expr{Expr: `"trusted_internal"`},
			SourceAuthority: &mapping.Expr{Expr: `"corroborating"`},
			ResourceType:    &mapping.Expr{Expr: `"event"`},
			Timestamps:      mapping.TimestampMappings{},
		},
	}
	_, err := mapping.Apply(spec, map[string]any{}, mapping.SourceCeilings{
		OrganizationID:        "org1",
		TrustCeiling:          "trusted_system",
		AuthorityCeiling:      "source_of_truth",
		ClassificationCeiling: "restricted",
	}, mapping.Options{})
	if err == nil {
		t.Fatal("expected unknown classification to be rejected")
	}
	ae, ok := platform.AsAPIError(err)
	if !ok || ae.HTTPStatus != 403 {
		t.Fatalf("want forbidden, got %v", err)
	}
	if mapping.RankClassification("top_secret") >= 0 {
		t.Fatal("unknown classification must rank < 0")
	}
}
