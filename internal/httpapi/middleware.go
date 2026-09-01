package httpapi

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xsama/context-fabric/internal/observability"
)

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("response writer does not support hijacking")
}

func (w *statusWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// MiddlewareConfig tunes the HTTP middleware stack.
type MiddlewareConfig struct {
	Logger              *slog.Logger
	MaxBodyBytes        int64
	IntakeBatchMaxBytes int64
}

// DefaultMiddlewareConfig returns production-safe defaults (1MB default body, 8MB intake batch).
func DefaultMiddlewareConfig() MiddlewareConfig {
	return MiddlewareConfig{
		Logger:              slog.Default(),
		MaxBodyBytes:        1 << 20,
		IntakeBatchMaxBytes: 8 << 20,
	}
}

// Middleware wraps next with request ID, security headers, panic recovery, access logging, and body limits.
func Middleware(next http.Handler, cfg MiddlewareConfig) http.Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1 << 20
	}
	if cfg.IntakeBatchMaxBytes <= 0 {
		cfg.IntakeBatchMaxBytes = 8 << 20
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if reqID == "" {
			reqID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", reqID)
		setSecurityHeaders(w)

		limit := cfg.MaxBodyBytes
		if isIntakeBatchRoute(r) {
			limit = cfg.IntakeBatchMaxBytes
		}
		if r.ContentLength > limit {
			http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
			return
		}
		if r.Body != nil && r.Body != http.NoBody {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if rec := recover(); rec != nil {
				cfg.Logger.Error("http panic recovered",
					"panic", rec,
					"path", r.URL.Path,
					"method", r.Method,
					"stack", string(debug.Stack()),
				)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(sw, r)

		observability.RecordHTTP(r.URL.Path, sw.status)
		cfg.Logger.Info("http request",
			"request_id", reqID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"bytes", sw.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
			"user_agent", r.UserAgent(),
		)
	})
}

func isIntakeBatchRoute(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/context/intake:batch")
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
}
