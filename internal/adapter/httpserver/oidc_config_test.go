package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnit_OIDCConfig_ReturnsEndpointsAsJSON(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	h := NewOIDCConfigHandler(OIDCConfigResponse{
		Issuer:                 "https://auth.example.com/realms/flow",
		DeviceAuthorizationURL: "https://auth.example.com/device",
		TokenURL:               "https://auth.example.com/token",
		ClientID:               "flow-cli",
	})
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/oidc/config", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", rr.Header().Get("Content-Type"))
	}
	var got OIDCConfigResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DeviceAuthorizationURL != "https://auth.example.com/device" {
		t.Errorf("DeviceAuthorizationURL = %q", got.DeviceAuthorizationURL)
	}
	if got.Issuer != "https://auth.example.com/realms/flow" {
		t.Errorf("Issuer = %q", got.Issuer)
	}
}

func TestUnit_OIDCConfig_EmptyResponse_StillValidJSON(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	h := NewOIDCConfigHandler(OIDCConfigResponse{})
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/oidc/config", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var got OIDCConfigResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
}
