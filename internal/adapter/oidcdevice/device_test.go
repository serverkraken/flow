package oidcdevice_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/adapter/oidcdevice"
)

// newProviderServer serves an OIDC discovery doc plus device + token endpoints.
func newProviderServer(t *testing.T) (*httptest.Server, *http.ServeMux) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                        srv.URL,
			"authorization_endpoint":        srv.URL + "/auth",
			"token_endpoint":                srv.URL + "/token",
			"device_authorization_endpoint": srv.URL + "/device/code",
			"jwks_uri":                      srv.URL + "/jwks",
		})
	})
	return srv, mux
}

func TestStartAndPoll(t *testing.T) {
	srv, mux := newProviderServer(t)
	mux.HandleFunc("/device/code", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "DEV-1",
			"user_code":        "ABCD-EFGH",
			"verification_uri": srv.URL + "/device",
			"expires_in":       300,
			"interval":         0,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "acc-tok",
			"refresh_token": "ref-tok",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})

	ctx := oidc.InsecureIssuerURLContext(context.Background(), srv.URL)
	fl, err := oidcdevice.New(ctx, srv.URL, "flow-cli")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	da, err := fl.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if da.UserCode != "ABCD-EFGH" {
		t.Fatalf("user code: %q", da.UserCode)
	}
	tok, err := fl.Poll(ctx, da)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if tok.AccessToken != "acc-tok" || tok.RefreshToken != "ref-tok" {
		t.Fatalf("token: %+v", tok)
	}
}
