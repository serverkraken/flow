package httpserver

import (
	"encoding/json"
	"net/http"
)

// NewMeHandler serves /api/v1/me — returns the current user's identity as
// derived from the session cookie or bearer token. Behind auth middleware.
func NewMeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv, ok := sessionFromContext(r.Context())
		if !ok {
			http.Error(w, "no session", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":   sv.Sub,
			"email": sv.Email,
			"name":  sv.Name,
		})
	})
}
