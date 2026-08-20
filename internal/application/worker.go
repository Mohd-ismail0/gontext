package app

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/xsama/context-fabric/internal/changes"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

const indexConsumer = "context-index-projector"

// Worker runs outbox relay and index projection consumers.
type Worker struct {
	Ledger     ports.LedgerStore
	Bus        ports.EventBus
	Index      ports.IndexProvider
	ChangeFeed *changes.Service
	Interval   time.Duration
	Batch      int

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
	w.wg.Add(3)
	go func() {
		defer w.wg.Done()
		w.relayLoop(ctx)
	}()
	go func() {
		defer w.wg.Done()
		w.consumeLoop(ctx)
	}()
	go func() {
		defer w.wg.Done()
		w.webhookLoop(ctx)
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
		// Project index from outbox regardless of bus type (NATS may not be consumed yet).
		if err := w.projectPayload(ctx, e.Payload); err != nil {
			log.Printf("worker project after relay: %v", err)
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

func (w *Worker) webhookLoop(ctx context.Context) {
	interval := w.Interval * 3
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
			if w.ChangeFeed == nil {
				continue
			}
			n := w.ChangeFeed.DeliverPending(ctx, nil, 5*time.Second, w.Batch)
			if n > 0 {
				log.Printf("worker webhooks: delivered %d", n)
			}
		}
	}
}

type jetStreamFetcher interface {
	FetchIndex(ctx context.Context, max int, wait time.Duration) ([]ports.EventMessage, error)
}

type outboxPayload struct {
	EventID    string `json:"event_id"`
	OrgID      string `json:"org_id"`
	ResourceID string `json:"resource_id"`
	RevisionID string `json:"revision_id"`
}

func (w *Worker) projectPayload(ctx context.Context, data []byte) error {
	var p outboxPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil
	}
	if p.OrgID == "" || p.EventID == "" {
		return nil
	}
	has, err := w.Ledger.HasInbox(ctx, p.OrgID, indexConsumer, p.EventID)
	if err != nil || has {
		return err
	}
	if w.Index != nil && p.ResourceID != "" {
		rec, err := w.Ledger.GetRecord(ctx, p.OrgID, p.ResourceID)
		if err != nil {
			return err
		}
		if err := w.Index.Upsert(ctx, []ports.IndexDocument{{
			ResourceID: p.ResourceID,
			RevisionID: p.RevisionID,
			OrgID:      p.OrgID,
			Text:       rec.Title,
			Labels:     rec.Labels,
			Attributes: map[string]string{"classification": rec.Classification},
		}}); err != nil {
			return err
		}
	}
	return w.Ledger.PutInbox(ctx, ports.InboxEntry{
		ID: platform.NewEventID(), OrgID: p.OrgID, Consumer: indexConsumer,
		MsgID: p.EventID, ProcessedAt: time.Now().UTC(),
	})
}

// consumeOnce projects from NoopBus buffers and optional JetStream pull consumers.
func (w *Worker) consumeOnce(ctx context.Context) error {
	if nb, ok := w.Bus.(interface{ Published() []ports.EventMessage }); ok {
		for _, msg := range nb.Published() {
			if err := w.projectPayload(ctx, msg.Data); err != nil {
				log.Printf("worker consume noop: %v", err)
			}
		}
	}
	if fetcher, ok := w.Bus.(jetStreamFetcher); ok {
		msgs, err := fetcher.FetchIndex(ctx, w.Batch, 500*time.Millisecond)
		if err != nil {
			return err
		}
		for _, msg := range msgs {
			if err := w.projectPayload(ctx, msg.Data); err != nil {
				log.Printf("worker consume jetstream: %v", err)
			}
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
func (noopBus) Subscribe(context.Context, string, func(msg ports.EventMessage) error) (ports.Subscription, error) {
	return noopSub{}, nil
}

type noopSub struct{}

func (noopSub) Unsubscribe() error { return nil }
