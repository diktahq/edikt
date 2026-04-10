// Package email is the application-facing email API. Email sending is
// always asynchronous: callers enqueue a job, the worker pool picks it up
// and talks to the SMTP / SES provider. Synchronous email is forbidden
// because a slow MTA must never block a user-facing request.
package email

import (
	"context"

	"github.com/example/orders-service/internal/queue"
)

// Sender is what handlers and services depend on. Use Sender, not the
// queue directly, so the type signatures stay narrow and self-explanatory.
type Sender struct {
	q *queue.Queue
}

// NewSender constructs a Sender that pushes jobs onto the given queue.
func NewSender(q *queue.Queue) *Sender {
	return &Sender{q: q}
}

// SendOrderConfirmation enqueues an "order confirmed" email for a user.
func (s *Sender) SendOrderConfirmation(ctx context.Context, userID, orderID string) error {
	return s.q.Enqueue(ctx, OrderConfirmationJob{UserID: userID, OrderID: orderID})
}

// OrderConfirmationJob is the payload the email worker consumes.
type OrderConfirmationJob struct {
	UserID  string
	OrderID string
}

// JobName implements queue.Job.
func (OrderConfirmationJob) JobName() string { return "email.order_confirmation" }
