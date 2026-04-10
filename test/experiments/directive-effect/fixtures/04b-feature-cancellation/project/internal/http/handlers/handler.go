// Package handlers contains the HTTP handlers for the orders service.
//
// Handlers are deliberately thin: they decode the request, call the
// service layer, and write the response. All business logic — tenant
// scoping, repository access, audit, events, payment, email — lives in
// the service layer.
package handlers

import "github.com/example/orders-service/internal/service"

// OrdersHandler delegates all order operations to the service layer.
type OrdersHandler struct {
	svc *service.OrdersService
}

// NewOrdersHandler constructs the handler bundle.
func NewOrdersHandler(svc *service.OrdersService) *OrdersHandler {
	return &OrdersHandler{svc: svc}
}
