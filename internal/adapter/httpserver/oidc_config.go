package httpserver

// OIDCConfigResponse exposes the IdP endpoints clients need to perform a
// device-flow. Full handler arrives in Task 17.
type OIDCConfigResponse struct {
	Issuer                 string `json:"issuer"`
	DeviceAuthorizationURL string `json:"device_authorization_endpoint"`
	TokenURL               string `json:"token_endpoint"`
	ClientID               string `json:"client_id"`
}
