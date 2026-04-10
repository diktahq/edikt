package handlers

import (
	"errors"
	"net/http"

	"github.com/example/orders-service/internal/web"
	"github.com/example/orders-service/internal/logging"
	"github.com/example/orders-service/internal/repository"
)

// GetUser handles GET /users/{id}.
func (h *UsersHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context())

	id := r.PathValue("id")
	if id == "" {
		web.WriteError(w, http.StatusBadRequest, "user id is required")
		return
	}

	user, err := h.repos.Users.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "user not found")
			return
		}
		log.Error("get user failed", "user_id", id, "err", err)
		web.WriteError(w, http.StatusInternalServerError, "could not load user")
		return
	}

	web.WriteJSON(w, http.StatusOK, user)
}
