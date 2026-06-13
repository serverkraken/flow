package oidcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ResolveEndpoints asks flow-server's /api/v1/oidc/config for the IdP's
// device + token endpoints, so clients never need the IdP URL directly.
// deviceURL may be empty (not all flows need it); tokenURL is always present
// or an error is returned.
func ResolveEndpoints(ctx context.Context, serverURL string, httpc *http.Client) (deviceURL, tokenURL string, err error) {
	if httpc == nil {
		httpc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/v1/oidc/config", nil)
	if err != nil {
		return "", "", err
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("oidc/config: status %s", resp.Status)
	}
	var cfg struct {
		DeviceURL string `json:"device_authorization_endpoint"`
		TokenURL  string `json:"token_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return "", "", err
	}
	if cfg.TokenURL == "" {
		return "", "", fmt.Errorf("oidc/config: no token_endpoint")
	}
	return cfg.DeviceURL, cfg.TokenURL, nil
}
