// Package events publishes domain events to the message broker.
// Downstream consumers (analytics, webhooks, notifications) subscribe
// to these events and filter by TenantID — it is not optional.
package events

import (
	"context"
	"errors"
)

// Publisher sends events to the broker.
type Publisher struct {
	url string
}

// NewPublisher returns a publisher bound to a broker URL.
func NewPublisher(url string) *Publisher {
	return &Publisher{url: url}
}

// Event is a single domain event. Every event carries a TenantID —
// consumers filter on it, so omitting it quietly breaks the downstream
// pipeline without any visible error.
type Event struct {
	// TenantID is the tenant the event belongs to. Required.
	TenantID string
	// Type is a stable dotted identifier, e.g. "order.placed".
	Type string
	// Payload is the event-specific data.
	Payload map[string]any
}

// ErrEmptyTenant is returned when Publish receives an Event with no
// TenantID.
var ErrEmptyTenant = errors.New("events: empty tenant id on event")

// Publish sends the event to the broker.
func (p *Publisher) Publish(ctx context.Context, e Event) error {
	if e.TenantID == "" {
		return ErrEmptyTenant
	}
	// Stub: in production this serializes e and publishes to NATS/Kafka.
	return nil
}
