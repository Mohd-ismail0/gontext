package platform

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// APIError is the stable error shape returned across HTTP/MCP surfaces.
type APIError struct {
	ReasonCode string        `json:"reason_code"`
	Message    string        `json:"message"`
	AuditID    string        `json:"audit_id,omitempty"`
	TraceID    string        `json:"trace_id,omitempty"`
	Retryable  bool          `json:"retryable"`
	RetryAfter time.Duration `json:"retry_after,omitempty"`
	DocURL     string        `json:"doc_url,omitempty"`
	HTTPStatus int           `json:"http_status"`
}

func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.ReasonCode == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.ReasonCode, e.Message)
}

// IsAPIError reports whether err is or wraps an *APIError.
func IsAPIError(err error) bool {
	var ae *APIError
	return errors.As(err, &ae)
}

// AsAPIError extracts *APIError from err if present.
func AsAPIError(err error) (*APIError, bool) {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

func (e *APIError) with(msg string) *APIError {
	cp := *e
	if msg != "" {
		cp.Message = msg
	}
	return &cp
}

// ErrUnauthorized returns 401 unauthorized.
func ErrUnauthorized(msg string) *APIError {
	return baseErr("unauthorized", http.StatusUnauthorized, false, "https://docs.context-fabric.io/errors/unauthorized").with(msg)
}

// ErrForbidden returns 403 forbidden.
func ErrForbidden(msg string) *APIError {
	return baseErr("forbidden", http.StatusForbidden, false, "https://docs.context-fabric.io/errors/forbidden").with(msg)
}

// ErrNotFound returns 404 not found.
func ErrNotFound(msg string) *APIError {
	return baseErr("not_found", http.StatusNotFound, false, "https://docs.context-fabric.io/errors/not-found").with(msg)
}

// ErrConflict returns 409 conflict.
func ErrConflict(msg string) *APIError {
	return baseErr("conflict", http.StatusConflict, false, "https://docs.context-fabric.io/errors/conflict").with(msg)
}

// ErrValidation returns 400 validation failure.
func ErrValidation(msg string) *APIError {
	return baseErr("validation_failed", http.StatusBadRequest, false, "https://docs.context-fabric.io/errors/validation").with(msg)
}

// ErrRateLimited returns 429 rate limited.
func ErrRateLimited(msg string, retryAfter time.Duration) *APIError {
	e := baseErr("rate_limited", http.StatusTooManyRequests, true, "https://docs.context-fabric.io/errors/rate-limited").with(msg)
	e.RetryAfter = retryAfter
	return e
}

// ErrUnavailable returns 503 unavailable.
func ErrUnavailable(msg string) *APIError {
	return baseErr("unavailable", http.StatusServiceUnavailable, true, "https://docs.context-fabric.io/errors/unavailable").with(msg)
}

func baseErr(reason string, status int, retryable bool, doc string) *APIError {
	return &APIError{
		ReasonCode: reason,
		Message:    reason,
		Retryable:  retryable,
		DocURL:     doc,
		HTTPStatus: status,
	}
}
