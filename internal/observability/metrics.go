// Package observability exposes lightweight process metrics for ops scrape.
package observability

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
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

	HTTPRequestsTotal = "http_requests_total"
	HTTPErrorsTotal   = "http_errors_total"

	// DependencyLatencyMS holds the last observed dependency round-trip in milliseconds.
	DependencyLatencyMS = "dependency_latency_ms"
	BuildInfo           = "build_info"
)

// Bounded label values for Prometheus series cardinality control.
var (
	routeClasses = map[string]struct{}{
		"health": {}, "api": {}, "mcp": {}, "metrics": {}, "other": {},
	}
	statusClasses = map[string]struct{}{
		"2xx": {}, "3xx": {}, "4xx": {}, "5xx": {},
	}
	errorClasses = map[string]struct{}{
		"client": {}, "server": {}, "dependency": {},
	}
	dependencyNames = map[string]struct{}{
		"postgres": {}, "s3": {}, "nats": {}, "openfga": {}, "oidc": {},
	}
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
	labeled = map[string]map[string]*atomic.Int64{}
	gauges  = map[string]*atomic.Int64{
		BuildInfo: {},
	}
	buildLabels = map[string]string{}
)

// BuildInfoLabels pins version metadata on the build_info gauge (value always 1).
type BuildInfoLabels struct {
	Version   string
	Commit    string
	GoVersion string
}

// SetBuildInfo records immutable build metadata for the build_info gauge.
func SetBuildInfo(info BuildInfoLabels) {
	mu.Lock()
	buildLabels = map[string]string{
		"version":    sanitizeLabel(info.Version),
		"commit":     sanitizeLabel(info.Commit),
		"go_version": sanitizeLabel(info.GoVersion),
	}
	mu.Unlock()
	gauges[BuildInfo].Store(1)
}

// RouteClass maps an HTTP path to a bounded route class label.
func RouteClass(path string) string {
	p := strings.TrimSpace(path)
	switch {
	case strings.HasPrefix(p, "/health"):
		return "health"
	case p == "/metrics":
		return "metrics"
	case p == "/mcp" || strings.HasPrefix(p, "/mcp/"):
		return "mcp"
	case strings.HasPrefix(p, "/v1/"):
		return "api"
	default:
		return "other"
	}
}

// StatusClass maps an HTTP status code to a bounded status class label.
func StatusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

// RecordHTTP increments request and error counters for one completed HTTP exchange.
func RecordHTTP(path string, status int) {
	route := RouteClass(path)
	statusClass := StatusClass(status)
	incLabeled(HTTPRequestsTotal, route, statusClass)
	if status >= 400 {
		errClass := "client"
		if status >= 500 {
			errClass = "server"
		}
		incLabeled(HTTPErrorsTotal, route, errClass)
	}
}

// RecordDependencyLatency stores the last observed dependency latency (milliseconds).
func RecordDependencyLatency(dependency string, ms int64) {
	if _, ok := dependencyNames[dependency]; !ok {
		return
	}
	mu.Lock()
	key := labeledKey(DependencyLatencyMS, dependency, "last")
	if labeled[DependencyLatencyMS] == nil {
		labeled[DependencyLatencyMS] = map[string]*atomic.Int64{}
	}
	c, ok := labeled[DependencyLatencyMS][key]
	if !ok {
		c = &atomic.Int64{}
		labeled[DependencyLatencyMS][key] = c
	}
	mu.Unlock()
	c.Store(ms)
}

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
	if !ok {
		c, ok = gauges[name]
	}
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
		labeledSnap := copyLabeled(labeled)
		buildSnap := copyStringMap(buildLabels)
		mu.Unlock()

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

		writeLabeledCounter(w, HTTPRequestsTotal, labeledSnap[HTTPRequestsTotal], "route_class", "status_class")
		writeLabeledCounter(w, HTTPErrorsTotal, labeledSnap[HTTPErrorsTotal], "route_class", "error_class")
		writeLabeledGauge(w, DependencyLatencyMS, labeledSnap[DependencyLatencyMS], "dependency", "window")

		if len(buildSnap) > 0 {
			labels := formatLabels(buildSnap)
			_, _ = fmt.Fprintf(w, "# HELP context_fabric_%s Context Fabric build metadata (value=1)\n", BuildInfo)
			_, _ = fmt.Fprintf(w, "# TYPE context_fabric_%s gauge\n", BuildInfo)
			_, _ = fmt.Fprintf(w, "context_fabric_%s{%s} 1\n", BuildInfo, labels)
		}
	})
}

func incLabeled(metric, labelA, labelB string) {
	if metric == HTTPRequestsTotal {
		if _, ok := routeClasses[labelA]; !ok {
			labelA = "other"
		}
		if _, ok := statusClasses[labelB]; !ok {
			labelB = "5xx"
		}
	}
	if metric == HTTPErrorsTotal {
		if _, ok := routeClasses[labelA]; !ok {
			labelA = "other"
		}
		if _, ok := errorClasses[labelB]; !ok {
			labelB = "server"
		}
	}
	mu.Lock()
	if labeled[metric] == nil {
		labeled[metric] = map[string]*atomic.Int64{}
	}
	key := labeledKey(metric, labelA, labelB)
	c, ok := labeled[metric][key]
	if !ok {
		c = &atomic.Int64{}
		labeled[metric][key] = c
	}
	mu.Unlock()
	c.Add(1)
}

func labeledKey(metric, a, b string) string {
	return metric + "|" + a + "|" + b
}

func copyLabeled(src map[string]map[string]*atomic.Int64) map[string]map[string]int64 {
	out := make(map[string]map[string]int64, len(src))
	for metric, series := range src {
		m := make(map[string]int64, len(series))
		for k, v := range series {
			m[k] = v.Load()
		}
		out[metric] = m
	}
	return out
}

func copyStringMap(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func writeLabeledCounter(w http.ResponseWriter, name string, series map[string]int64, labelA, labelB string) {
	if len(series) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "# HELP context_fabric_%s Context Fabric metric %s\n", name, name)
	_, _ = fmt.Fprintf(w, "# TYPE context_fabric_%s counter\n", name)
	keys := make([]string, 0, len(series))
	for k := range series {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts := strings.Split(k, "|")
		if len(parts) != 3 {
			continue
		}
		labels := formatLabels(map[string]string{labelA: parts[1], labelB: parts[2]})
		_, _ = fmt.Fprintf(w, "context_fabric_%s{%s} %d\n", name, labels, series[k])
	}
}

func writeLabeledGauge(w http.ResponseWriter, name string, series map[string]int64, labelA, labelB string) {
	if len(series) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "# HELP context_fabric_%s Last observed dependency latency in milliseconds\n", name)
	_, _ = fmt.Fprintf(w, "# TYPE context_fabric_%s gauge\n", name)
	keys := make([]string, 0, len(series))
	for k := range series {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts := strings.Split(k, "|")
		if len(parts) != 3 {
			continue
		}
		labels := formatLabels(map[string]string{labelA: parts[1], labelB: parts[2]})
		_, _ = fmt.Fprintf(w, "context_fabric_%s{%s} %d\n", name, labels, series[k])
	}
}

func formatLabels(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, sanitizeLabel(m[k])))
	}
	return strings.Join(parts, ",")
}

func sanitizeLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	v = strings.ReplaceAll(v, `"`, `'`)
	v = strings.ReplaceAll(v, "\n", " ")
	return v
}

func writeMetric(w http.ResponseWriter, name, kind string, v int64) {
	_, _ = fmt.Fprintf(w, "# HELP context_fabric_%s Context Fabric metric %s\n", name, name)
	_, _ = fmt.Fprintf(w, "# TYPE context_fabric_%s %s\n", name, kind)
	_, _ = fmt.Fprintf(w, "context_fabric_%s %d\n", name, v)
}
