// Package handlers contains HTTP handlers for the example API.
// Each handler extracts the authoritative tenant ID from the request
// context (set by internal/middleware/tenant.go) and passes it to the
// repository layer, which enforces tenant isolation in every query.
package handlers

import (
	"encoding/json"
	"net/http"
)

// GetUser returns the user record for the given ID, scoped to the
// authenticated tenant. Bypassing the tenant filter would leak data
// across tenants, so this handler MUST read the tenant from the
// request context (never from the request body or query parameters).
func GetUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	tenantID, _ := r.Context().Value("tenantID").(string)
	if tenantID == "" {
		http.Error(w, "missing tenant context", http.StatusUnauthorized)
		return
	}

	// In a real implementation, this calls a user repository method
	// that scopes the query by tenant_id. The fixture uses a canned
	// response to keep the example minimal.
	user := map[string]string{
		"id":        userID,
		"tenant_id": tenantID,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}
