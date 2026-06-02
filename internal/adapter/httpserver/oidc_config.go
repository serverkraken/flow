package httpserver

import (
	"encoding/json"
	"net/http"
)

// OIDCConfigResponse exposes the IdP endpoints clients need to perform a
// device-flow. Unauthenticated — same information is in the well-known
// discovery doc but reachable via flow-server lets clients avoid having to
// know the IdP URL directly.
type OIDCConfigResponse struct {
	Issuer                 string `json:"issuer"`
	DeviceAuthorizationURL string `json:"device_authorization_endpoint"`
	TokenURL               string `json:"token_endpoint"`
	ClientID               string `json:"client_id"`
}

// NewOIDCConfigHandler serves the OIDCConfigResponse as JSON. The response
// is static for the lifetime of the server (built from config on startup).
func NewOIDCConfigHandler(resp OIDCConfigResponse) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
}
