// Package logging provides a structured logger that automatically enriches
// every log event with request-scoped fields read from context.Context.
//
// Always obtain a logger via FromContext(ctx) inside handlers, jobs, and
// repository methods — never construct one directly. The contextual logger
// pulls request_id and tenant_id from the context and stamps them on every
// event, which is how the platform-wide log search remains correlatable.
package logging

import (
	"context"
	"log/slog"
	"os"

	"github.com/example/orders-service/internal/ctxkeys"
)

// Logger is a thin wrapper around slog.Logger that carries context-scoped
// fields. The zero value is not safe to use; obtain via New or FromContext.
type Logger struct {
	inner *slog.Logger
}

var base = slog.New(slog.NewJSONHandler(os.Stdout, nil))

// New constructs a logger pre-populated with context fields.
func New(ctx context.Context) *Logger {
	l := base
	if v, ok := ctx.Value(ctxkeys.RequestID).(string); ok && v != "" {
		l = l.With("request_id", v)
	}
	if v, ok := ctx.Value(ctxkeys.TenantID).(string); ok && v != "" {
		l = l.With("tenant_id", v)
	}
	if v, ok := ctx.Value(ctxkeys.UserID).(string); ok && v != "" {
		l = l.With("user_id", v)
	}
	return &Logger{inner: l}
}

// FromContext returns a logger pre-bound to the request scope.
func FromContext(ctx context.Context) *Logger { return New(ctx) }

func (l *Logger) Info(msg string, args ...any)  { l.inner.Info(msg, args...) }
func (l *Logger) Warn(msg string, args ...any)  { l.inner.Warn(msg, args...) }
func (l *Logger) Error(msg string, args ...any) { l.inner.Error(msg, args...) }
