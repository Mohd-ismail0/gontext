package quota

import (
	"fmt"
	"sync"
	"time"

	"github.com/xsama/context-fabric/internal/platform"
)

// Operation identifies a rate-limited surface.
type Operation string

const (
	OpSearch Operation = "search"
	OpIntake Operation = "intake"
	OpExport Operation = "export"
)

// Key scopes a bucket to org, optional source, and principal.
type Key struct {
	OrgID       string
	SourceID    string
	PrincipalID string
	Op          Operation
}

func (k Key) String() string {
	return fmt.Sprintf("%s|%s|%s|%s", k.OrgID, k.SourceID, k.PrincipalID, k.Op)
}

// Limits configures refill rates (tokens per minute) and burst capacity.
type Limits struct {
	SearchPerMinute float64
	IntakePerMinute float64
	ExportPerMinute float64
	Burst           float64
}

// DefaultLimits returns conservative in-memory defaults.
func DefaultLimits() Limits {
	return Limits{
		SearchPerMinute: 60,
		IntakePerMinute: 120,
		ExportPerMinute: 10,
		Burst:           20,
	}
}

// Limiter is an in-memory token-bucket quota enforcer.
type Limiter struct {
	mu      sync.Mutex
	limits  Limits
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	tokens     float64
	capacity   float64
	refillRate float64 // tokens per second
	last       time.Time
}

// NewLimiter creates an in-memory quota limiter.
func NewLimiter(limits Limits) *Limiter {
	if limits.Burst <= 0 {
		limits.Burst = 1
	}
	return &Limiter{
		limits:  limits,
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
}

// Allow consumes one token for the given key or returns ErrRateLimited.
func (l *Limiter) Allow(key Key) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	rate := l.rateFor(key.Op)
	if rate <= 0 {
		return nil
	}
	id := key.String()
	b, ok := l.buckets[id]
	now := l.now()
	if !ok {
		b = &bucket{
			tokens:     l.limits.Burst,
			capacity:   l.limits.Burst,
			refillRate: rate / 60.0,
			last:       now,
		}
		l.buckets[id] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.refillRate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.last = now
	}
	if b.tokens < 1 {
		retry := time.Duration((1 - b.tokens) / b.refillRate * float64(time.Second))
		if retry < time.Millisecond {
			retry = time.Millisecond
		}
		return platform.ErrRateLimited("quota exceeded for "+string(key.Op), retry)
	}
	b.tokens--
	return nil
}

func (l *Limiter) rateFor(op Operation) float64 {
	switch op {
	case OpSearch:
		return l.limits.SearchPerMinute
	case OpIntake:
		return l.limits.IntakePerMinute
	case OpExport:
		return l.limits.ExportPerMinute
	default:
		return 0
	}
}
