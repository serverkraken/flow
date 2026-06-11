// internal/adapter/httpserver/meta.go
package httpserver

import "net/http"

// MetaResponse is the §7 version handshake. Public — no secrets, must be
// reachable before login so clients can warn about version skew.
type MetaResponse struct {
	ServerVersion    string `json:"server_version"`
	MinClientVersion string `json:"min_client_version"`
}

// NewMetaHandler returns the GET /api/v1/meta handler.
func NewMetaHandler(meta MetaResponse) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, meta)
	})
}
