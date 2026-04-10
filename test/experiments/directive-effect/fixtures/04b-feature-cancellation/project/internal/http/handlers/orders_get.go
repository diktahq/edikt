package handlers

import (
	"errors"
	"net/http"

	"github.com/example/orders-service/internal/service"
	"github.com/example/orders-service/internal/web"
)

// GetOrder handles GET /orders/{id}.
func (h *OrdersHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		web.WriteError(w, http.StatusBadRequest, "order id is required")
		return
	}

	order, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "order not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "could not load order")
		return
	}

	web.WriteJSON(w, http.StatusOK, order)
}
