package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

var redactedKeys = map[string]struct{}{
	"password":           {},
	"secret":             {},
	"token":              {},
	"authorization":      {},
	"api_key":            {},
	"apikey":             {},
	"webhook_signing":    {},
	"deletion_signing":   {},
	"client_secret":      {},
	"postgres_admin_dsn": {},
	"dsn":                {},
}

// SetupLogging configures the default slog logger with JSON output, redaction, and LOG_LEVEL.
// Supports LOG_LEVEL and CONTEXT_FABRIC_LOG_LEVEL (debug|info|warn|error).
func SetupLogging() *slog.Logger {
	level := parseLogLevel(firstNonEmptyEnv("LOG_LEVEL", "CONTEXT_FABRIC_LOG_LEVEL"))
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	logger := slog.New(&redactingHandler{next: base})
	slog.SetDefault(logger)
	return logger
}

type redactingHandler struct {
	next slog.Handler
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, redactAttr(a))
		return true
	})
	r2 := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r2.AddAttrs(attrs...)
	return h.next.Handle(ctx, r2)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = redactAttr(a)
	}
	return &redactingHandler{next: h.next.WithAttrs(out)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: h.next.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	for part := range redactedKeys {
		if strings.Contains(key, part) {
			return slog.String(a.Key, "[REDACTED]")
		}
	}
	if a.Value.Kind() == slog.KindString {
		val := a.Value.String()
		if looksSensitive(key, val) {
			return slog.String(a.Key, "[REDACTED]")
		}
	}
	return a
}

func looksSensitive(key, val string) bool {
	if strings.TrimSpace(val) == "" {
		return false
	}
	lower := strings.ToLower(val)
	if strings.Contains(lower, "postgres://") || strings.Contains(lower, "nats://") {
		return strings.Contains(key, "dsn") || strings.Contains(key, "url") && strings.Contains(key, "admin")
	}
	if strings.HasPrefix(strings.ToLower(key), "authorization") {
		return true
	}
	return false
}

func parseLogLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func firstNonEmptyEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
