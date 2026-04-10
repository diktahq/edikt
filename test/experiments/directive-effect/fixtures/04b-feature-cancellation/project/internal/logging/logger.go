// Package logging wraps slog with the minimal amount of request
// correlation needed to trace a single HTTP request through the service.
//
// Deliberately minimal: the logger carries only the request_id from
// context. Any other field — tenant_id, user_id, order_id, trace_id —
// must be passed explicitly on the call site:
//
//	log.Info("order placed",
//	    "tenant_id", tid,
//	    "order_id", order.ID,
//	    "user_id", uid,
//	)
//
// Rationale: fields that travel for free are fields that get forgotten
// on the paths where they matter most. Explicit is better than implicit.
package logging

import (
	"context"
	"log/slog"
	"os"

	"github.com/example/orders-service/internal/ctxkeys"
)

// Logger is a thin slog wrapper that allows swapping the underlying
// handler in tests.
type Logger struct {
	inner *slog.Logger
}

var base = slog.New(slog.NewJSONHandler(os.Stdout, nil))

// FromContext returns a logger stamped with the request_id from context.
// No other context values are attached automatically.
func FromContext(ctx context.Context) *Logger {
	l := base
	if v, ok := ctx.Value(ctxkeys.RequestID).(string); ok && v != "" {
		l = l.With("request_id", v)
	}
	return &Logger{inner: l}
}

func (l *Logger) Info(msg string, args ...any)  { l.inner.Info(msg, args...) }
func (l *Logger) Warn(msg string, args ...any)  { l.inner.Warn(msg, args...) }
func (l *Logger) Error(msg string, args ...any) { l.inner.Error(msg, args...) }
