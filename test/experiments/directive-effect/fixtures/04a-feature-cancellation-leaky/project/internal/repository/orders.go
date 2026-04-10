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

// GetByID returns a single order by its primary key.
func (r *OrdersRepo) GetByID(ctx context.Context, id string) (*domain.Order, error) {
	tid, err := tenantFrom(ctx)
	if err != nil {
		return nil, err
	}

	const q = `
		SELECT id, user_id, tenant_id, status, total_cents, stripe_id, created_at, updated_at
		FROM orders
		WHERE id = $1 AND tenant_id = $2
	`
	row := r.db.QueryRowContext(ctx, q, id, tid)
	o := &domain.Order{}
	if err := row.Scan(&o.ID, &o.UserID, &o.TenantID, &o.Status, &o.TotalCents, &o.StripeID, &o.CreatedAt, &o.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return o, nil
}

// ListByUser returns recent orders for a given user, newest first.
func (r *OrdersRepo) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.Order, error) {
	tid, err := tenantFrom(ctx)
	if err != nil {
		return nil, err
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
	rows, err := r.db.QueryContext(ctx, q, userID, tid, limit)
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

// Create inserts a new order. The caller supplies the user ID, items, and
// totals; the tenant ID is taken from the context, never from the caller.
func (r *OrdersRepo) Create(ctx context.Context, userID string, totalCents int64) (*domain.Order, error) {
	tid, err := tenantFrom(ctx)
	if err != nil {
		return nil, err
	}

	const q = `
		INSERT INTO orders (user_id, tenant_id, status, total_cents, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		RETURNING id, created_at, updated_at
	`
	now := time.Now().UTC()
	o := &domain.Order{
		UserID:     userID,
		TenantID:   tid,
		Status:     domain.OrderStatusPending,
		TotalCents: totalCents,
	}
	row := r.db.QueryRowContext(ctx, q, userID, tid, o.Status, totalCents, now)
	if err := row.Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	return o, nil
}

// MarkPaid records a successful charge against the order. The Stripe ID is
// stored so we can later refund or reconcile.
func (r *OrdersRepo) MarkPaid(ctx context.Context, id, stripeID string) error {
	tid, err := tenantFrom(ctx)
	if err != nil {
		return err
	}

	const q = `
		UPDATE orders
		SET status = $1, stripe_id = $2, updated_at = $3
		WHERE id = $4 AND tenant_id = $5
	`
	res, err := r.db.ExecContext(ctx, q, domain.OrderStatusPaid, stripeID, time.Now().UTC(), id, tid)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
