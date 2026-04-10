// Package web holds small HTTP response helpers shared by every handler.
// Helpers in this file standardise response formatting so that internal
// error details never leak to the client.
package web

import (
	"encoding/json"
	"net/http"
)

// WriteJSON serialises v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a sanitised error response. The detail string is the
// only thing the client sees — internal error wrapping, stack traces, and
// SQL errors must NEVER reach this function.
func WriteError(w http.ResponseWriter, status int, detail string) {
	WriteJSON(w, status, map[string]string{"error": detail})
}
