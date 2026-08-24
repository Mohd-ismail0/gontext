// Package observability exposes lightweight process metrics for ops scrape.
package observability

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
)

// Names of counters exposed on /metrics.
const (
	SearchRequests    = "search_requests"
	AuthzBatchChecks  = "authz_batch_checks"
	GraphRequests     = "graph_requests"
	OutboxPending     = "outbox_pending"
	WebhookDeliveries = "webhook_deliveries"
)

var (
	mu       sync.Mutex
	counters = map[string]*atomic.Int64{
		SearchRequests:    {},
		AuthzBatchChecks:  {},
		GraphRequests:     {},
		OutboxPending:     {},
		WebhookDeliveries: {},
	}
)

// Inc increments a named counter by 1.
func Inc(name string) {
	Add(name, 1)
}

// Add increments a named counter by delta (ignored if the name is unknown).
func Add(name string, delta int64) {
	mu.Lock()
	c, ok := counters[name]
	mu.Unlock()
	if !ok || c == nil {
		return
	}
	c.Add(delta)
}

// SetGauge sets an absolute gauge-like counter value (e.g. outbox_pending).
func SetGauge(name string, value int64) {
	mu.Lock()
	c, ok := counters[name]
	mu.Unlock()
	if !ok || c == nil {
		return
	}
	c.Store(value)
}

// Get returns the current value of a counter.
func Get(name string) int64 {
	mu.Lock()
	c, ok := counters[name]
	mu.Unlock()
	if !ok || c == nil {
		return 0
	}
	return c.Load()
}

// Handler returns a Prometheus text exposition handler for GET /metrics.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		mu.Lock()
		names := make([]string, 0, len(counters))
		for n := range counters {
			names = append(names, n)
		}
		mu.Unlock()
		// Stable-ish order for scrape diffs.
		order := []string{SearchRequests, AuthzBatchChecks, GraphRequests, OutboxPending, WebhookDeliveries}
		seen := map[string]bool{}
		for _, name := range order {
			seen[name] = true
			kind := "counter"
			if name == OutboxPending {
				kind = "gauge"
			}
			writeMetric(w, name, kind, Get(name))
		}
		for _, name := range names {
			if seen[name] {
				continue
			}
			writeMetric(w, name, "counter", Get(name))
		}
	})
}

func writeMetric(w http.ResponseWriter, name, kind string, v int64) {
	_, _ = fmt.Fprintf(w, "# HELP context_fabric_%s Context Fabric metric %s\n", name, name)
	_, _ = fmt.Fprintf(w, "# TYPE context_fabric_%s %s\n", name, kind)
	_, _ = fmt.Fprintf(w, "context_fabric_%s %d\n", name, v)
}
