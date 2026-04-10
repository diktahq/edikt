// Package payment wraps the third-party payment provider (Stripe).
// State-changing calls require an idempotency key so retried requests
// after transient failures cannot double-charge or double-refund.
package payment

import (
	"context"
	"errors"
	"fmt"
)

// StripeClient is the narrow Stripe interface used by the service layer.
type StripeClient struct {
	secret string
}

// NewStripeClient constructs a client bound to a secret API key.
func NewStripeClient(secret string) *StripeClient {
	return &StripeClient{secret: secret}
}

// ChargeResult is what Charge returns on success.
type ChargeResult struct {
	StripeID string
}

// Charge runs a payment. idempotencyKey is required.
func (c *StripeClient) Charge(ctx context.Context, amountCents int64, idempotencyKey string) (*ChargeResult, error) {
	if idempotencyKey == "" {
		return nil, errors.New("payment: idempotency key required")
	}
	if amountCents <= 0 {
		return nil, errors.New("payment: amount must be positive")
	}
	return &ChargeResult{StripeID: fmt.Sprintf("ch_stub_%s", idempotencyKey)}, nil
}

// RefundResult is what Refund returns on success.
type RefundResult struct {
	StripeID string
}

// Refund reverses a previous charge. idempotencyKey is required.
func (c *StripeClient) Refund(ctx context.Context, chargeID string, idempotencyKey string) (*RefundResult, error) {
	if idempotencyKey == "" {
		return nil, errors.New("payment: idempotency key required")
	}
	if chargeID == "" {
		return nil, errors.New("payment: charge id required")
	}
	return &RefundResult{StripeID: fmt.Sprintf("re_stub_%s", idempotencyKey)}, nil
}
