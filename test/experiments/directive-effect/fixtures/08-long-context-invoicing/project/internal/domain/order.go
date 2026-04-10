package domain

import "time"

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusFulfilled OrderStatus = "fulfilled"
)

type Order struct {
	ID         string
	UserID     string
	TenantID   string
	Status     OrderStatus
	TotalCents int64
	StripeID   string
	CreatedAt  time.Time
}
