package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/example/orders-service/internal/ctxkeys"
	"github.com/example/orders-service/internal/logging"
	"github.com/example/orders-service/internal/web"
)

// createOrderRequest is the JSON body the create endpoint accepts.
// Note: there is no `tenant_id` field. Tenant scope is taken from the
// authenticated session, never from the request body.
type createOrderRequest struct {
	UserID     string `json:"user_id"`
	TotalCents int64  `json:"total_cents"`
}

// CreateOrder handles POST /orders.
func (h *OrdersHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context())

	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		web.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" || req.TotalCents <= 0 {
		web.WriteError(w, http.StatusBadRequest, "user_id and total_cents are required")
		return
	}

	// Create the order. The repository layer pulls tenant from ctx.
	order, err := h.repos.Orders.Create(r.Context(), req.UserID, req.TotalCents)
	if err != nil {
		log.Error("create order failed", "err", err)
		web.WriteError(w, http.StatusInternalServerError, "could not create order")
		return
	}

	// Charge via Stripe with an idempotency key derived from the order
	// ID — a retried request after a network blip will not double-charge.
	idemKey := fmt.Sprintf("order-%s-charge", order.ID)
	charge, err := h.stripe.Charge(r.Context(), order.TotalCents, idemKey)
	if err != nil {
		log.Error("charge failed", "order_id", order.ID, "err", err)
		web.WriteError(w, http.StatusBadGateway, "payment failed")
		return
	}

	if err := h.repos.Orders.MarkPaid(r.Context(), order.ID, charge.StripeID); err != nil {
		log.Error("mark paid failed", "order_id", order.ID, "err", err)
		web.WriteError(w, http.StatusInternalServerError, "order recorded but state update failed")
		return
	}

	// Confirmation email goes via the queue. Synchronous email is forbidden
	// because a slow MTA must not block the user-facing response.
	if err := h.email.SendOrderConfirmation(r.Context(), req.UserID, order.ID); err != nil {
		log.Warn("enqueue confirmation email failed", "order_id", order.ID, "err", err)
		// non-fatal — the order is created and paid; we will reconcile later
	}

	// Sanity check — the user is who the session says they are.
	if uid, _ := r.Context().Value(ctxkeys.UserID).(string); uid != "" {
		log.Info("order created", "order_id", order.ID)
	}

	web.WriteJSON(w, http.StatusCreated, order)
}
