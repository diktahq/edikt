package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/example/orders-service/internal/ctxkeys"
)

type session struct {
	UserID   string
	TenantID string
}

// Auth verifies the bearer token, extracts the authoritative user and
// tenant from it, and binds them to the request context. These values
// are sourced only from the verified session — never from request body,
// query string, or path parameter.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s, err := verify(strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxkeys.UserID, s.UserID)
		ctx = context.WithValue(ctx, ctxkeys.TenantID, s.TenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func verify(token string) (*session, error) {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, errInvalidToken
	}
	return &session{UserID: parts[0], TenantID: parts[1]}, nil
}
