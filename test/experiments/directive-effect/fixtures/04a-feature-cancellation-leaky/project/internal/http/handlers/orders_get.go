package handlers

import (
	"errors"
	"net/http"

	"github.com/example/orders-service/internal/web"
	"github.com/example/orders-service/internal/logging"
	"github.com/example/orders-service/internal/repository"
)

// GetOrder handles GET /orders/{id}.
func (h *OrdersHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context())

	id := r.PathValue("id")
	if id == "" {
		web.WriteError(w, http.StatusBadRequest, "order id is required")
		return
	}

	order, err := h.repos.Orders.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "order not found")
			return
		}
		log.Error("get order failed", "order_id", id, "err", err)
		web.WriteError(w, http.StatusInternalServerError, "could not load order")
		return
	}

	web.WriteJSON(w, http.StatusOK, order)
}
