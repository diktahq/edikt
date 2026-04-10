// Package handlers contains the HTTP handlers for the orders service.
//
// Each handler:
//   - obtains a logger via logging.FromContext (which automatically carries
//     request_id and tenant_id)
//   - delegates database access to the repository layer (never raw SQL)
//   - enqueues third-party calls (email, webhooks) instead of running them
//     synchronously on the request path
//   - returns sanitized error messages to the client (apphttp.WriteError)
//     and logs the underlying details server-side
package handlers

import (
	"github.com/example/orders-service/internal/email"
	"github.com/example/orders-service/internal/payment"
	"github.com/example/orders-service/internal/repository"
)

// OrdersHandler bundles the dependencies needed by the order endpoints.
type OrdersHandler struct {
	repos  *repository.Repos
	stripe *payment.StripeClient
	email  *email.Sender
}

// NewOrdersHandler constructs the handler bundle.
func NewOrdersHandler(repos *repository.Repos, stripe *payment.StripeClient, mailer *email.Sender) *OrdersHandler {
	return &OrdersHandler{repos: repos, stripe: stripe, email: mailer}
}

// UsersHandler is a much smaller bundle for user-related endpoints.
type UsersHandler struct {
	repos *repository.Repos
}

// NewUsersHandler constructs the user handler bundle.
func NewUsersHandler(repos *repository.Repos) *UsersHandler {
	return &UsersHandler{repos: repos}
}
