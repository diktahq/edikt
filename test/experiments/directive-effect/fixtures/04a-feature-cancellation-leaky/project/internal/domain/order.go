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

// Order is the persistence model for a customer order. The TenantID field
// exists on the row but is never populated from request input — it is set
// by the repository from the request context.
type Order struct {
	ID          string
	UserID      string
	TenantID    string
	Status      OrderStatus
	TotalCents  int64
	StripeID    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// LineItem is one product line on an order.
type LineItem struct {
	SKU       string
	Quantity  int
	UnitCents int64
}
