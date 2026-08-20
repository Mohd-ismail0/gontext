package audit

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// Logger is an append-only audit sink.
// Implementations must never persist raw queries, content, or tokens.
type Logger interface {
	Append(ctx context.Context, event ports.AuditEvent) error
}

// MemoryLogger stores audit events in process memory (tests/demo).
type MemoryLogger struct {
	mu     sync.Mutex
	events []ports.AuditEvent
}

// NewMemory returns an in-memory append-only audit logger.
func NewMemory() *MemoryLogger {
	return &MemoryLogger{events: make([]ports.AuditEvent, 0, 64)}
}

var _ Logger = (*MemoryLogger)(nil)

// Append stores a sanitized audit event.
func (m *MemoryLogger) Append(_ context.Context, event ports.AuditEvent) error {
	sanitize(&event)
	if event.AuditID == "" {
		event.AuditID = platform.NewEventID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

// Events returns a copy of stored events.
func (m *MemoryLogger) Events() []ports.AuditEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ports.AuditEvent, len(m.events))
	copy(out, m.events)
	return out
}

// LedgerLogger persists audit events through a ledger-backed writer.
type LedgerWriter interface {
	AppendAudit(ctx context.Context, event ports.AuditEvent) error
}

// LedgerLogger adapts a ledger writer to Logger.
type LedgerLogger struct {
	Writer LedgerWriter
}

var _ Logger = (*LedgerLogger)(nil)

// Append forwards a sanitized event to the ledger.
func (l *LedgerLogger) Append(ctx context.Context, event ports.AuditEvent) error {
	if l.Writer == nil {
		return platform.ErrUnavailable("ledger audit writer not configured")
	}
	sanitize(&event)
	if event.AuditID == "" {
		event.AuditID = platform.NewEventID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	return l.Writer.AppendAudit(ctx, event)
}

// forbiddenAttrKeys must never appear in audit attributes.
var forbiddenAttrKeys = map[string]struct{}{
	"query": {}, "q": {}, "content": {}, "body": {}, "token": {},
	"access_token": {}, "refresh_token": {}, "authorization": {},
	"api_key": {}, "password": {}, "secret": {}, "raw": {},
}

func sanitize(e *ports.AuditEvent) {
	if e.Attributes == nil {
		return
	}
	clean := make(map[string]string, len(e.Attributes))
	for k, v := range e.Attributes {
		if _, bad := forbiddenAttrKeys[strings.ToLower(k)]; bad {
			continue
		}
		clean[k] = v
	}
	e.Attributes = clean
	// Cap sample size to avoid content-like dumps.
	const maxSample = 16
	if len(e.ResourceIDsSample) > maxSample {
		e.ResourceIDsSample = e.ResourceIDsSample[:maxSample]
	}
}
