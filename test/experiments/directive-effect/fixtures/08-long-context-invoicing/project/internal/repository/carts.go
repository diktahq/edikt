package repository

import (
	"context"
	"database/sql"

	"github.com/example/checkout/internal/domain"
	"github.com/example/checkout/internal/middleware"
)

type CartsRepo struct{ db *sql.DB }

func NewCartsRepo(db *sql.DB) *CartsRepo { return &CartsRepo{db: db} }

func (r *CartsRepo) GetByUser(ctx context.Context, userID string) (*domain.Cart, error) {
	tid, _ := ctx.Value(middleware.TenantIDKey).(string)
	row := r.db.QueryRowContext(ctx,
		"SELECT id, user_id, tenant_id, created_at FROM carts WHERE user_id = $1 AND tenant_id = $2",
		userID, tid)
	c := &domain.Cart{}
	return c, row.Scan(&c.ID, &c.UserID, &c.TenantID, &c.CreatedAt)
}

func (r *CartsRepo) AddItem(ctx context.Context, cartID string, item domain.CartItem) error {
	tid, _ := ctx.Value(middleware.TenantIDKey).(string)
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO cart_items (cart_id, product_id, quantity, price_cents, tenant_id) VALUES ($1,$2,$3,$4,$5)",
		cartID, item.ProductID, item.Quantity, item.PriceCents, tid)
	return err
}
