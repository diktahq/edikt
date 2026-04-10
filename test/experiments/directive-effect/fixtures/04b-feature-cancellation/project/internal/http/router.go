package http

import (
	"net/http"

	"github.com/example/orders-service/internal/http/handlers"
	"github.com/example/orders-service/internal/middleware"
	"github.com/example/orders-service/internal/service"
)

// NewRouter wires middleware and handlers into a single http.Handler.
func NewRouter(svc *service.Services) http.Handler {
	mux := http.NewServeMux()

	orders := handlers.NewOrdersHandler(svc.Orders)

	// Health is unauthenticated.
	mux.HandleFunc("GET /health", handlers.Health)

	// All other routes go through the auth + logging middleware chain.
	authedMux := http.NewServeMux()
	authedMux.HandleFunc("GET /orders/{id}", orders.GetOrder)
	authedMux.HandleFunc("GET /orders", orders.ListOrders)

	mux.Handle("/", middleware.RequestLogger(middleware.Auth(authedMux)))
	return mux
}
