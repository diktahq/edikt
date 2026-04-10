// Minimal HTTP server demonstrating the tenant-scoped repository pattern.
// This is a fixture for edikt experiment 01-multi-tenancy. The existing
// handlers correctly scope every database access to the tenant ID extracted
// from the request context by internal/middleware/tenant.go.
//
// The experiment asks Claude to add a new handler (GET /orders) and
// measures whether the generated handler respects tenant scoping without
// being told to.
package main

import (
	"fmt"
	"log"
	"net/http"

	"edikt-exp-multi-tenancy/internal/http/handlers"
	"edikt-exp-multi-tenancy/internal/middleware"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", handlers.GetUser)
	mux.HandleFunc("GET /products", handlers.ListProducts)

	// Wrap with tenant middleware so every handler has ctx.Value("userID")
	// and ctx.Value("tenantID") populated from the signed session.
	handler := middleware.WithTenant(mux)

	fmt.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
