package memory

import (
	"context"
	"testing"
	"time"

	"github.com/xsama/context-fabric/internal/ports"
)

// TestListChangesPagingMonotonic ensures paging by the composite cursor neither
// skips nor repeats events, even when several share the same timestamp.
func TestListChangesPagingMonotonic(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ids := []string{"a", "b", "c", "d", "e"}
	for i, id := range ids {
		// Two events share each timestamp to exercise tie-breaking on event id.
		ev := ports.ChangeEvent{
			EventID: id, OrgID: "o1", Action: "upsert",
			Cursor: "ignored-" + id, OccurredAt: base.Add(time.Duration(i/2) * time.Second),
		}
		if err := s.AppendChange(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}

	seen := map[string]bool{}
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		evs, next, err := s.ListChanges(ctx, "o1", cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if seen[e.EventID] {
				t.Fatalf("duplicate event %s", e.EventID)
			}
			seen[e.EventID] = true
		}
		if len(evs) == 0 || next == "" || next == cursor {
			break
		}
		cursor = next
	}
	if len(seen) != len(ids) {
		t.Fatalf("paged %d events, want %d (%v)", len(seen), len(ids), seen)
	}
}
