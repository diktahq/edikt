package service

import (
	"context"
	"errors"
	"github.com/example/orders-service/internal/audit"
	"github.com/example/orders-service/internal/domain"
	"github.com/example/orders-service/internal/email"
	"github.com/example/orders-service/internal/events"
	"github.com/example/orders-service/internal/logging"
	"github.com/example/orders-service/internal/payment"
	"github.com/example/orders-service/internal/repository"
)

// OrdersService is the business-logic entry point for order operations.
type OrdersService struct {
	repo    *repository.OrdersRepo
	stripe  *payment.StripeClient
	audit   *audit.Recorder
	events  *events.Publisher
	email   *email.Sender
}

// NewOrdersService constructs the service from its collaborators.
func NewOrdersService(
	repo *repository.OrdersRepo,
	stripe *payment.StripeClient,
	auditor *audit.Recorder,
	pub *events.Publisher,
	mailer *email.Sender,
) *OrdersService {
	return &OrdersService{
		repo:   repo,
		stripe: stripe,
		audit:  auditor,
		events: pub,
		email:  mailer,
	}
}

// GetByID returns the specified order if it is owned by the authenticated
// user within the authenticated tenant.
func (s *OrdersService) GetByID(ctx context.Context, orderID string) (*domain.Order, error) {
	tid, uid, err := scope(ctx)
	if err != nil {
		return nil, err
	}
	log := logging.FromContext(ctx)

	order, err := s.repo.GetByID(ctx, tid, orderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		log.Error("get order failed",
			"tenant_id", tid,
			"order_id", orderID,
			"err", err,
		)
		return nil, err
	}

	if order.UserID != uid {
		// Do not leak existence — return NotFound rather than Forbidden.
		return nil, ErrNotFound
	}

	return order, nil
}

// List returns the authenticated user's recent orders.
func (s *OrdersService) List(ctx context.Context, limit int) ([]*domain.Order, error) {
	tid, uid, err := scope(ctx)
	if err != nil {
		return nil, err
	}
	log := logging.FromContext(ctx)

	orders, err := s.repo.ListByUser(ctx, tid, uid, limit)
	if err != nil {
		log.Error("list orders failed",
			"tenant_id", tid,
			"user_id", uid,
			"err", err,
		)
		return nil, err
	}
	return orders, nil
}
