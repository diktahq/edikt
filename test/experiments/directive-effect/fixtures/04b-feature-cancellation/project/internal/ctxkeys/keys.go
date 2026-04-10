// Package ctxkeys defines the typed keys used to store request-scoped values
// on context.Context.
package ctxkeys

type ctxKey int

const (
	// UserID is the authenticated user's ID, set by the auth middleware
	// after verifying the session token.
	UserID ctxKey = iota

	// TenantID is the tenant the authenticated user belongs to, set by
	// the auth middleware. It is the only authoritative source of
	// tenant scope for the request.
	TenantID

	// RequestID is a per-request UUID set by the logging middleware so
	// that log lines can be correlated end-to-end.
	RequestID
)
