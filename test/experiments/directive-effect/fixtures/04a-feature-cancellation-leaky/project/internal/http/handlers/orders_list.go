package handlers

import (
	"net/http"
	"strconv"

	"github.com/example/orders-service/internal/web"
	"github.com/example/orders-service/internal/logging"
)

// ListOrders handles GET /users/{user_id}/orders.
func (h *OrdersHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context())

	userID := r.PathValue("user_id")
	if userID == "" {
		web.WriteError(w, http.StatusBadRequest, "user id is required")
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	orders, err := h.repos.Orders.ListByUser(r.Context(), userID, limit)
	if err != nil {
		log.Error("list orders failed", "user_id", userID, "err", err)
		web.WriteError(w, http.StatusInternalServerError, "could not list orders")
		return
	}

	web.WriteJSON(w, http.StatusOK, orders)
}
