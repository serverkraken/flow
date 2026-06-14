package oidcauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAndAuthCodeURL(t *testing.T) {
	mux := http.NewServeMux()
	var issuer string
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 issuer,
			"authorization_endpoint": issuer + "/authorize",
			"token_endpoint":         issuer + "/token",
			"jwks_uri":               issuer + "/jwks",
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	issuer = ts.URL

	a, err := New(context.Background(), issuer, "flow", "secret", "https://app/auth/callback")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	url := a.AuthCodeURL("xyz")
	if !strings.Contains(url, "/authorize") || !strings.Contains(url, "state=xyz") || !strings.Contains(url, "client_id=flow") {
		t.Fatalf("AuthCodeURL malformed: %s", url)
	}
}
