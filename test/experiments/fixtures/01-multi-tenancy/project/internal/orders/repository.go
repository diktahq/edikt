// Package orders provides tenant-scoped database access for Order records.
//
// IMPORTANT: Every method on this repository requires an explicit tenantID
// parameter. The repository injects a tenant_id filter into every SQL query
// so that tenant isolation is enforced automatically. Callers that need to
// access orders MUST go through this repository — raw SQL elsewhere in the
// codebase is forbidden.
package orders

import (
	"context"
	"database/sql"
	"fmt"
)

// Order represents a single order row.
type Order struct {
	ID       string
	UserID   string
	TenantID string
	Total    int64 // cents
}

// Repository provides tenant-scoped access to the orders table.
type Repository struct {
	db *sql.DB
}

// NewRepository constructs a Repository wrapping the given sql.DB.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// FindOrderByID returns the single order with the given ID, scoped to the
// given tenant. Returns sql.ErrNoRows if the order doesn't exist OR if it
// exists but belongs to a different tenant.
func (r *Repository) FindOrderByID(ctx context.Context, tenantID, orderID string) (*Order, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	row := r.db.QueryRowContext(ctx,
		"SELECT id, user_id, tenant_id, total FROM orders WHERE id = $1 AND tenant_id = $2",
		orderID, tenantID,
	)

	var o Order
	if err := row.Scan(&o.ID, &o.UserID, &o.TenantID, &o.Total); err != nil {
		return nil, err
	}
	return &o, nil
}

// FindOrdersByUserAndTenant returns all orders for the given user in the
// given tenant. Both arguments are required — omitting tenant_id would
// leak data across tenants.
func (r *Repository) FindOrdersByUserAndTenant(ctx context.Context, userID, tenantID string) ([]Order, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	rows, err := r.db.QueryContext(ctx,
		"SELECT id, user_id, tenant_id, total FROM orders WHERE user_id = $1 AND tenant_id = $2 ORDER BY id DESC",
		userID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.UserID, &o.TenantID, &o.Total); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
