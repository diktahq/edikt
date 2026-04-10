// Package middleware provides the tenant context extraction middleware.
//
// WithTenant extracts the signed session from the Authorization header,
// verifies the signature, and binds the user ID and tenant ID to the
// request context. Handlers downstream read the IDs from the context
// via ctx.Value("userID") and ctx.Value("tenantID").
//
// The tenant ID is authoritative — it MUST NOT be accepted from the
// request body or query parameters. Only the signed session determines
// which tenant the request belongs to.
package middleware

import (
	"context"
	"net/http"
)

type contextKey string

const (
	// UserIDKey is the context key for the authenticated user ID.
	UserIDKey contextKey = "userID"
	// TenantIDKey is the context key for the authoritative tenant ID.
	TenantIDKey contextKey = "tenantID"
)

// WithTenant extracts userID and tenantID from the signed session and binds
// them to the request context. Requests without a valid session are rejected
// at this layer before reaching any handler.
func WithTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// In a real implementation, this parses a JWT or session cookie.
		// The fixture uses hardcoded values to keep the example minimal.
		sessionToken := r.Header.Get("Authorization")
		if sessionToken == "" {
			http.Error(w, "missing session", http.StatusUnauthorized)
			return
		}

		userID := "user-123"
		tenantID := "tenant-456"

		ctx := r.Context()
		ctx = context.WithValue(ctx, UserIDKey, userID)
		ctx = context.WithValue(ctx, TenantIDKey, tenantID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
