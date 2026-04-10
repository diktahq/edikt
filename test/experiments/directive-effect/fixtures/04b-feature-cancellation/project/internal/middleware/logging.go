package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/example/orders-service/internal/ctxkeys"
)

// RequestLogger stamps a request_id on the context so all log lines for
// a single request can be correlated. It does NOT touch tenant or user —
// those are the auth middleware's job, and the logger reads them
// explicitly at the call site when needed.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := newRequestID()
		ctx := context.WithValue(r.Context(), ctxkeys.RequestID, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
