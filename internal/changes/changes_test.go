package changes_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	"github.com/xsama/context-fabric/internal/changes"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestWebhookEventHasNoContentFields(t *testing.T) {
	store := memory.NewStore()
	svc := changes.New(store, []byte("test-secret"))
	org := "org_chg_1"
	sub, err := svc.UpsertWebhook(org, "https://example.com/hook", []string{"resource.accepted"}, "hook-secret")
	if err != nil {
		t.Fatal(err)
	}
	ev := ports.ChangeEvent{
		EventID: "evt1", OrgID: org, ResourceID: "r1", RevisionID: "rev1",
		Action: "resource.accepted", Cursor: "c1", OccurredAt: time.Now().UTC(),
	}
	body, sig, delivery, err := svc.DeliverSigned(org, sub.ID, ev)
	if err != nil {
		t.Fatal(err)
	}
	if sig == "" || !delivery.Success {
		t.Fatal("expected signed delivery")
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatal(err)
	}
	if changes.HasContentFields(obj) {
		t.Fatalf("webhook payload must be metadata-only; got %v", obj)
	}
	for _, forbidden := range []string{"content", "vector", "embedding", "prompt", "authorization_tuples"} {
		if _, ok := obj[forbidden]; ok {
			t.Fatalf("forbidden field %s present", forbidden)
		}
	}
	if obj["resource_id"] != "r1" || obj["event_id"] != "evt1" {
		t.Fatalf("missing required metadata: %v", obj)
	}
}
