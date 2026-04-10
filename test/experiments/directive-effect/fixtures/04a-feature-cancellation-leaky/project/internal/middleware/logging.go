package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/example/orders-service/internal/ctxkeys"
	"github.com/example/orders-service/internal/logging"
)

// RequestLogger attaches a request-scoped structured logger to the context.
// Downstream handlers must obtain the logger via logging.FromContext rather
// than constructing their own — the contextual logger automatically includes
// the request ID and the tenant ID on every log line.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := newRequestID()
		ctx := context.WithValue(r.Context(), ctxkeys.RequestID, reqID)

		log := logging.New(ctx)
		started := time.Now()

		next.ServeHTTP(w, r.WithContext(ctx))

		log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
