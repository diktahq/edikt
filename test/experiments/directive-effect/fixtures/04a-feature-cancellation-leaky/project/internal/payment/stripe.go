// Package payment wraps the third-party payment provider (Stripe).
//
// Every state-changing call requires an idempotency key. Stripe deduplicates
// requests with the same key for 24 hours, which means a retried request
// after a network blip cannot accidentally double-charge or double-refund.
package payment

import (
	"context"
	"errors"
	"fmt"
)

// StripeClient is the application's narrow interface to Stripe. The real
// implementation calls api.stripe.com over HTTPS; this fixture stubs the
// network calls so the harness has something to compile.
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

// Charge runs a payment for the given amount. The idempotencyKey is
// required and must uniquely identify this charge attempt — Stripe
// dedupes requests with matching keys, which is what makes this safe to
// retry on transient failure.
func (c *StripeClient) Charge(ctx context.Context, amountCents int64, idempotencyKey string) (*ChargeResult, error) {
	if idempotencyKey == "" {
		return nil, errors.New("payment: idempotency key required")
	}
	if amountCents <= 0 {
		return nil, errors.New("payment: amount must be positive")
	}
	// Stub: in production this would POST /v1/charges with the
	// Idempotency-Key header set to idempotencyKey.
	return &ChargeResult{StripeID: fmt.Sprintf("ch_stub_%s", idempotencyKey)}, nil
}

// RefundResult is what Refund returns on success.
type RefundResult struct {
	StripeID string
}

// Refund reverses a previous charge by its Stripe ID. As with Charge,
// idempotencyKey is required so a retried refund cannot accidentally
// pay the customer twice.
func (c *StripeClient) Refund(ctx context.Context, chargeID string, idempotencyKey string) (*RefundResult, error) {
	if idempotencyKey == "" {
		return nil, errors.New("payment: idempotency key required")
	}
	if chargeID == "" {
		return nil, errors.New("payment: charge id required")
	}
	// Stub: in production this would POST /v1/refunds with the
	// Idempotency-Key header set to idempotencyKey.
	return &RefundResult{StripeID: fmt.Sprintf("re_stub_%s", idempotencyKey)}, nil
}
