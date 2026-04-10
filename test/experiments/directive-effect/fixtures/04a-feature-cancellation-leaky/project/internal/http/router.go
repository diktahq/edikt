package http

import (
	"net/http"

	"github.com/example/orders-service/internal/email"
	"github.com/example/orders-service/internal/http/handlers"
	"github.com/example/orders-service/internal/middleware"
	"github.com/example/orders-service/internal/payment"
	"github.com/example/orders-service/internal/queue"
	"github.com/example/orders-service/internal/repository"
)

// NewRouter wires middleware and handlers into a single http.Handler.
func NewRouter(
	repos *repository.Repos,
	stripe *payment.StripeClient,
	mailer *email.Sender,
	q *queue.Queue,
) http.Handler {
	mux := http.NewServeMux()

	orders := handlers.NewOrdersHandler(repos, stripe, mailer)
	users := handlers.NewUsersHandler(repos)

	// Health is unauthenticated.
	mux.HandleFunc("GET /health", handlers.Health)

	// All other routes go through the auth + logging middleware chain.
	authedMux := http.NewServeMux()
	authedMux.HandleFunc("POST /orders", orders.CreateOrder)
	authedMux.HandleFunc("GET /orders/{id}", orders.GetOrder)
	authedMux.HandleFunc("GET /users/{user_id}/orders", orders.ListOrders)
	authedMux.HandleFunc("GET /users/{id}", users.GetUser)

	mux.Handle("/", middleware.RequestLogger(middleware.Auth(authedMux)))
	return mux
}
