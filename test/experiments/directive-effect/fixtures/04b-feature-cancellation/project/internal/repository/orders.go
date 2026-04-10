package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/example/orders-service/internal/domain"
)

// OrdersRepo provides database access for the orders table.
type OrdersRepo struct {
	db *sql.DB
}

// GetByID returns a single order for the given tenant.
func (r *OrdersRepo) GetByID(ctx context.Context, tenantID, id string) (*domain.Order, error) {
	if tenantID == "" {
		return nil, ErrEmptyTenant
	}

	const q = `
		SELECT id, user_id, tenant_id, status, total_cents, stripe_id, created_at, updated_at
		FROM orders
		WHERE id = $1 AND tenant_id = $2
	`
	row := r.db.QueryRowContext(ctx, q, id, tenantID)
	o := &domain.Order{}
	if err := row.Scan(&o.ID, &o.UserID, &o.TenantID, &o.Status, &o.TotalCents, &o.StripeID, &o.CreatedAt, &o.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return o, nil
}

// ListByUser returns a user's recent orders within the given tenant.
func (r *OrdersRepo) ListByUser(ctx context.Context, tenantID, userID string, limit int) ([]*domain.Order, error) {
	if tenantID == "" {
		return nil, ErrEmptyTenant
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	const q = `
		SELECT id, user_id, tenant_id, status, total_cents, stripe_id, created_at, updated_at
		FROM orders
		WHERE user_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC
		LIMIT $3
	`
	rows, err := r.db.QueryContext(ctx, q, userID, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.Order
	for rows.Next() {
		o := &domain.Order{}
		if err := rows.Scan(&o.ID, &o.UserID, &o.TenantID, &o.Status, &o.TotalCents, &o.StripeID, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Insert creates a new order in the given tenant.
func (r *OrdersRepo) Insert(ctx context.Context, tenantID, userID string, totalCents int64) (*domain.Order, error) {
	if tenantID == "" {
		return nil, ErrEmptyTenant
	}

	const q = `
		INSERT INTO orders (user_id, tenant_id, status, total_cents, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		RETURNING id, created_at, updated_at
	`
	now := time.Now().UTC()
	o := &domain.Order{
		UserID:     userID,
		TenantID:   tenantID,
		Status:     domain.OrderStatusPending,
		TotalCents: totalCents,
	}
	row := r.db.QueryRowContext(ctx, q, userID, tenantID, o.Status, totalCents, now)
	if err := row.Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	return o, nil
}

// MarkPaid records a successful charge against the order.
func (r *OrdersRepo) MarkPaid(ctx context.Context, tenantID, id, stripeID string) error {
	if tenantID == "" {
		return ErrEmptyTenant
	}

	const q = `
		UPDATE orders
		SET status = $1, stripe_id = $2, updated_at = $3
		WHERE id = $4 AND tenant_id = $5
	`
	res, err := r.db.ExecContext(ctx, q, domain.OrderStatusPaid, stripeID, time.Now().UTC(), id, tenantID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
