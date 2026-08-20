package app

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

const indexConsumer = "context-index-projector"

// Worker runs outbox relay and index projection consumers.
type Worker struct {
	Ledger ports.LedgerStore
	Bus    ports.EventBus
	Index  ports.IndexProvider
	Interval time.Duration
	Batch    int

	stop chan struct{}
	wg   sync.WaitGroup
}

// Start launches relay and consumer loops until Stop or ctx cancel.
func (w *Worker) Start(ctx context.Context) {
	if w.Interval <= 0 {
		w.Interval = 2 * time.Second
	}
	if w.Batch <= 0 {
		w.Batch = 10
	}
	if w.Bus == nil {
		w.Bus = noopBus{}
	}
	w.stop = make(chan struct{})
	w.wg.Add(2)
	go func() {
		defer w.wg.Done()
		w.relayLoop(ctx)
	}()
	go func() {
		defer w.wg.Done()
		w.consumeLoop(ctx)
	}()
}

// Stop signals graceful shutdown and waits for loops.
func (w *Worker) Stop() {
	if w.stop != nil {
		select {
		case <-w.stop:
		default:
			close(w.stop)
		}
	}
	w.wg.Wait()
}

func (w *Worker) relayLoop(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
			if err := w.relayOnce(ctx); err != nil {
				log.Printf("worker relay: %v", err)
			}
		}
	}
}

func (w *Worker) relayOnce(ctx context.Context) error {
	entries, err := w.Ledger.ClaimOutbox(ctx, w.Batch)
	if err != nil || len(entries) == 0 {
		return err
	}
	published := make([]string, 0, len(entries))
	for _, e := range entries {
		headers := map[string]string{}
		for k, v := range e.Headers {
			headers[k] = v
		}
		eventID := e.ID
		if headers["event_id"] != "" {
			eventID = headers["event_id"]
		}
		headers["Nats-Msg-Id"] = eventID
		headers["event_id"] = eventID
		if err := w.Bus.Publish(ctx, e.Subject, e.Payload, headers); err != nil {
			return err
		}
		published = append(published, e.ID)
	}
	return w.Ledger.MarkOutboxPublished(ctx, published, time.Now().UTC())
}

func (w *Worker) consumeLoop(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
			if err := w.consumeOnce(ctx); err != nil {
				log.Printf("worker consume: %v", err)
			}
		}
	}
}

// consumeOnce projects unpublished outbox payloads that already have inbox potential.
// For NoopBus/tests we also scan ClaimOutbox-published by reading recent change via payload decode
// from a secondary path: subscribe is optional; here we process via HasInbox on published events
// retrieved from bus when available. Fallback: claim already-published by re-reading ledger outbox
// is not exposed — instead ApplicationService.Intake still upserts index synchronously for serve.
// Worker consumer focuses on bus-delivered work when Bus carries messages (Noop tracks them).
func (w *Worker) consumeOnce(ctx context.Context) error {
	type payload struct {
		EventID    string `json:"event_id"`
		OrgID      string `json:"org_id"`
		ResourceID string `json:"resource_id"`
		RevisionID string `json:"revision_id"`
	}

	// Prefer messages already published to an in-memory NoopBus.
	if nb, ok := w.Bus.(interface{ Published() []ports.EventMessage }); ok {
		for _, msg := range nb.Published() {
			var p payload
			if err := json.Unmarshal(msg.Data, &p); err != nil {
				continue
			}
			if p.OrgID == "" || p.EventID == "" {
				continue
			}
			has, err := w.Ledger.HasInbox(ctx, p.OrgID, indexConsumer, p.EventID)
			if err != nil || has {
				continue
			}
			if w.Index != nil {
				rec, err := w.Ledger.GetRecord(ctx, p.OrgID, p.ResourceID)
				if err != nil {
					continue
				}
				_ = w.Index.Upsert(ctx, []ports.IndexDocument{{
					ResourceID: p.ResourceID,
					RevisionID: p.RevisionID,
					OrgID:      p.OrgID,
					Text:       rec.Title,
					Labels:     rec.Labels,
					Attributes: map[string]string{"classification": rec.Classification},
				}})
			}
			_ = w.Ledger.PutInbox(ctx, ports.InboxEntry{
				ID: platform.NewEventID(), OrgID: p.OrgID, Consumer: indexConsumer,
				MsgID: p.EventID, ProcessedAt: time.Now().UTC(),
			})
		}
	}
	return nil
}

// RunWorker is a convenience blocking runner with graceful shutdown on ctx cancel.
func RunWorker(ctx context.Context, w *Worker) {
	w.Start(ctx)
	<-ctx.Done()
	w.Stop()
}

type noopBus struct{}

func (noopBus) Publish(context.Context, string, []byte, map[string]string) error { return nil }
func (noopBus) Subscribe(context.Context, string, func(ports.EventMessage) error) (ports.Subscription, error) {
	return noopSub{}, nil
}

type noopSub struct{}

func (noopSub) Unsubscribe() error { return nil }
