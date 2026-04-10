package domain

import "time"

type Cart struct {
	ID        string
	UserID    string
	TenantID  string
	Items     []CartItem
	CreatedAt time.Time
}

type CartItem struct {
	ProductID string
	Quantity  int
	PriceCents int64
}
