package handlers

import (
	"net/http"
	"strconv"

	"github.com/example/orders-service/internal/web"
)

// ListOrders handles GET /orders.
func (h *OrdersHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	orders, err := h.svc.List(r.Context(), limit)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "could not list orders")
		return
	}

	web.WriteJSON(w, http.StatusOK, orders)
}
