package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/example/checkout/internal/middleware"
	"github.com/example/checkout/internal/repository"
)

type CheckoutService struct {
	carts  *repository.CartsRepo
	orders *repository.OrdersRepo
}

func NewCheckoutService(carts *repository.CartsRepo, orders *repository.OrdersRepo) *CheckoutService {
	return &CheckoutService{carts: carts, orders: orders}
}

func (s *CheckoutService) Checkout(ctx context.Context) error {
	uid, _ := ctx.Value(middleware.UserIDKey).(string)
	tid, _ := ctx.Value(middleware.TenantIDKey).(string)

	cart, err := s.carts.GetByUser(ctx, uid)
	if err != nil {
		slog.Error("failed to load cart", "err", err)
		return err
	}

	if len(cart.Items) == 0 {
		return fmt.Errorf("cart is empty")
	}

	var total int64
	for _, item := range cart.Items {
		total += item.PriceCents * int64(item.Quantity)
	}

	// Stub: charge via Stripe
	stripeID := fmt.Sprintf("ch_%s", cart.ID)

	order, err := s.orders.Create(ctx, uid, total, stripeID)
	if err != nil {
		slog.Error("failed to create order", "tenant_id", tid, "err", err)
		return err
	}

	slog.Info("checkout complete", "order_id", order.ID, "tenant_id", tid, "total", total)
	return nil
}
