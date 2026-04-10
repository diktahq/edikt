package middleware

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey int

const (
	UserIDKey   ctxKey = iota
	TenantIDKey
)

func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		parts := strings.SplitN(token, ":", 2)
		if len(parts) != 2 {
			http.Error(w, "unauthorized", 401)
			return
		}
		ctx := context.WithValue(r.Context(), UserIDKey, parts[0])
		ctx = context.WithValue(ctx, TenantIDKey, parts[1])
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
