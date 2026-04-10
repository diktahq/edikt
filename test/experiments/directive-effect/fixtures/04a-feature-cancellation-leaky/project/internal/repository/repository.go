// Package repository is the only path to the database. Every method here
// reads the authoritative tenant ID from the request context and includes
// it in the SQL filter — callers do not pass tenant identifiers explicitly.
//
// New methods added to this package must follow the same convention.
package repository

import (
	"context"
	"database/sql"

	"github.com/example/orders-service/internal/ctxkeys"
)

// Repos bundles the per-table repositories that the rest of the service
// depends on. Construct via New.
type Repos struct {
	Orders *OrdersRepo
	Users  *UsersRepo
}

// New constructs the repository bundle around an open *sql.DB.
func New(db *sql.DB) *Repos {
	return &Repos{
		Orders: &OrdersRepo{db: db},
		Users:  &UsersRepo{db: db},
	}
}

// tenantFrom is a small helper used by every repository method. It returns
// the tenant ID from the context or ErrNoTenantContext if it is missing.
// Keeping this in one place makes the convention auditable.
func tenantFrom(ctx context.Context) (string, error) {
	v, ok := ctx.Value(ctxkeys.TenantID).(string)
	if !ok || v == "" {
		return "", ErrNoTenantContext
	}
	return v, nil
}
