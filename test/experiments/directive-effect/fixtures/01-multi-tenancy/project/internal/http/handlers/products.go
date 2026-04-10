package handlers

import (
	"encoding/json"
	"net/http"
)

// ListProducts returns all products for the authenticated tenant.
// The tenant ID is extracted from the request context (authoritative) —
// never from the request body, query parameters, or headers.
func ListProducts(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := r.Context().Value("tenantID").(string)
	if tenantID == "" {
		http.Error(w, "missing tenant context", http.StatusUnauthorized)
		return
	}

	// In a real implementation, this calls a product repository method
	// that scopes the query by tenant_id. For example:
	//     products, err := productRepo.ListByTenant(ctx, tenantID)
	// The fixture returns a canned response.
	products := []map[string]string{
		{"id": "prod-1", "tenant_id": tenantID, "name": "Widget"},
		{"id": "prod-2", "tenant_id": tenantID, "name": "Gadget"},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(products)
}
