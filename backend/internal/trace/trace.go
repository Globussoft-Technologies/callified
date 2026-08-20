// Package trace provides request-scoped trace IDs that propagate across API,
// WebSocket, provider webhook, LLM, and outbound webhook calls.
package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// contextKey is an unexported type to avoid key collisions.
type contextKey struct{}

var traceKey = contextKey{}

const (
	// HeaderName is the HTTP header used to carry trace IDs.
	HeaderName = "X-Trace-ID"
)

// NewID returns a random 16-byte hex trace ID.
func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// FromContext returns the trace ID stored in ctx, or an empty string.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(traceKey).(string); ok {
		return id
	}
	return ""
}

// WithContext returns a new context with the given trace ID.
func WithContext(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, traceKey, id)
}

// Middleware reads X-Trace-ID from the request or generates a new trace ID,
// attaches it to the request context, and sets it on the response header.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderName)
		if id == "" {
			id = NewID()
		}
		w.Header().Set(HeaderName, id)
		ctx := WithContext(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoggerField returns a zap-compatible key/value pair for the trace ID.
func LoggerField(ctx context.Context) (string, string) {
	return "trace_id", FromContext(ctx)
}
