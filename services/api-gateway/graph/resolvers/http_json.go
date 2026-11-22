package resolvers

import (
	"encoding/json"
	"net/http"
)

// writeJSON sends v as the JSON response body with the given status. The REST
// handlers in this package all answer this way; each used to carry its own
// copy of these two helpers.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr sends {"error": msg}, the shape every client reads errors from. An
// empty message falls back to the status text.
func writeErr(w http.ResponseWriter, code int, msg string) {
	if msg == "" {
		msg = http.StatusText(code)
	}
	writeJSON(w, code, map[string]string{"error": msg})
}
