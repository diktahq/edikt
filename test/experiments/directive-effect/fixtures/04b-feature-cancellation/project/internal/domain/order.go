// Package domain holds the core data types shared across the service.
package domain

import "time"

// OrderStatus enumerates the lifecycle states of an order.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusFulfilled OrderStatus = "fulfilled"
	OrderStatusCancelled OrderStatus = "cancelled"
	OrderStatusRefunded  OrderStatus = "refunded"
)

// Order is the persistence model for a customer order.
type Order struct {
	ID         string
	UserID     string
	TenantID   string
	Status     OrderStatus
	TotalCents int64
	StripeID   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
