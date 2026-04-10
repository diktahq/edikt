// Package email is the application-facing email API. Email sending is
// always asynchronous — callers enqueue a job, the worker pool talks to
// the SMTP provider. Synchronous email is forbidden.
package email

import (
	"context"

	"github.com/example/orders-service/internal/queue"
)

// Sender is what the service layer depends on.
type Sender struct {
	q *queue.Queue
}

// NewSender constructs a Sender that pushes jobs onto the given queue.
func NewSender(q *queue.Queue) *Sender {
	return &Sender{q: q}
}

// SendOrderConfirmation enqueues an "order confirmed" email.
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
