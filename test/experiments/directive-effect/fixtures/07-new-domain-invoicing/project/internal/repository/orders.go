package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/example/checkout/internal/domain"
	"github.com/example/checkout/internal/middleware"
)

type OrdersRepo struct{ db *sql.DB }

func NewOrdersRepo(db *sql.DB) *OrdersRepo { return &OrdersRepo{db: db} }

func (r *OrdersRepo) Create(ctx context.Context, userID string, totalCents int64, stripeID string) (*domain.Order, error) {
	tid, _ := ctx.Value(middleware.TenantIDKey).(string)
	o := &domain.Order{
		UserID:     userID,
		TenantID:   tid,
		Status:     domain.OrderStatusPaid,
		TotalCents: totalCents,
		StripeID:   stripeID,
		CreatedAt:  time.Now().UTC(),
	}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO orders (user_id, tenant_id, status, total_cents, stripe_id, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		o.UserID, o.TenantID, o.Status, o.TotalCents, o.StripeID, o.CreatedAt).Scan(&o.ID)
	return o, err
}

func (r *OrdersRepo) GetByID(ctx context.Context, id string) (*domain.Order, error) {
	tid, _ := ctx.Value(middleware.TenantIDKey).(string)
	o := &domain.Order{}
	err := r.db.QueryRowContext(ctx,
		"SELECT id, user_id, tenant_id, status, total_cents, stripe_id, created_at FROM orders WHERE id = $1 AND tenant_id = $2",
		id, tid).Scan(&o.ID, &o.UserID, &o.TenantID, &o.Status, &o.TotalCents, &o.StripeID, &o.CreatedAt)
	return o, err
}
