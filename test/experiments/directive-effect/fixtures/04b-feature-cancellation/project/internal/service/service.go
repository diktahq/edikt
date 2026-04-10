// Package service holds the business logic of the application.
//
// Every service method follows the same shape:
//
//  1. scope(ctx) — extract the authoritative tenant ID and user ID
//     from the request context. Every call downstream takes tenant
//     as an explicit argument; the service layer is the single point
//     where ctx → tenantID translation happens.
//
//  2. Call the repository with explicit tenantID. The repository does
//     not read context and does not know what tenant the caller is in.
//
//  3. Record an audit entry (audit.Entry.TenantID must be set).
//
//  4. Publish a domain event (events.Event.TenantID must be set).
//
//  5. Emit structured log lines with "tenant_id" as a field on every
//     call. The logger does not auto-populate; the caller does.
//
package service

import (
	"github.com/example/orders-service/internal/audit"
	"github.com/example/orders-service/internal/email"
	"github.com/example/orders-service/internal/events"
	"github.com/example/orders-service/internal/payment"
	"github.com/example/orders-service/internal/repository"
)

// Services bundles the service-layer entry points.
type Services struct {
	Orders *OrdersService
}

// New constructs the full service bundle from its collaborators.
func New(
	repos *repository.Repos,
	stripe *payment.StripeClient,
	auditor *audit.Recorder,
	pub *events.Publisher,
	mailer *email.Sender,
) *Services {
	return &Services{
		Orders: NewOrdersService(repos.Orders, stripe, auditor, pub, mailer),
	}
}
