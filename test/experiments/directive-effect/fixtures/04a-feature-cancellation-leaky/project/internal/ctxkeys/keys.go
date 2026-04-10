// Package ctxkeys defines the typed keys used to store request-scoped values
// on context.Context. Keep additions rare; every new key is one more thing
// every middleware and handler may need to know about.
package ctxkeys

type ctxKey int

const (
	// UserID is the authenticated user's ID, set by the auth middleware
	// after verifying the session token.
	UserID ctxKey = iota

	// TenantID is the tenant the authenticated user belongs to, set by the
	// auth middleware. It is the only authoritative source of tenant scope
	// for the request — the request body and URL parameters are never
	// trusted.
	TenantID

	// RequestID is a per-request UUID set by the logging middleware so that
	// log lines can be correlated end-to-end.
	RequestID
)
