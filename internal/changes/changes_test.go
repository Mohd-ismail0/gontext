package changes_test

import (
	"context"
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

func TestWebhookDuplicateAndDLQ(t *testing.T) {
	store := memory.NewStore()
	svc := changes.New(store, []byte("test-secret"))
	svc.MaxAttempts = 2
	clock := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return clock }

	org := "org_chg_dlq"
	sub, err := svc.UpsertWebhook(org, "http://127.0.0.1:9/nope", []string{"*"}, "hook-secret")
	if err != nil {
		t.Fatal(err)
	}
	ev := ports.ChangeEvent{
		EventID: "evt-dlq", OrgID: org, ResourceID: "r1", RevisionID: "rev1",
		Action: "resource.accepted", Cursor: "c1", OccurredAt: clock,
	}
	ctx := context.Background()
	for i := 0; i < svc.MaxAttempts; i++ {
		_ = svc.EnqueueAndAttempt(ctx, sub.ID, ev)
		clock = clock.Add(2 * time.Hour) // advance past exponential backoff
	}
	dlq := svc.ListDLQ(org, 20)
	if len(dlq) == 0 {
		t.Fatal("expected dead-letter entries after exhausting MaxAttempts")
	}
	body, _, _, err := svc.DeliverSigned(org, sub.ID, ports.ChangeEvent{
		EventID: "evt-ok", OrgID: org, ResourceID: "r2", RevisionID: "rev2",
		Action: "resource.indexed", Cursor: "c2", OccurredAt: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	dup := svc.RecordDuplicate(org, sub.ID, "evt-ok", body)
	if dup.Status != changes.StatusDuplicateIgnored {
		t.Fatalf("expected duplicate_ignored, got %s", dup.Status)
	}
}
