package oidcclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/adapter/oidcclient"
	"github.com/serverkraken/flow/internal/ports"
)

// setupRefreshServer creates a combined httptest.Server that serves:
//   - GET  /api/v1/oidc/config → token_endpoint pointing back at itself
//   - POST /token              → fresh access + refresh tokens
func setupRefreshServer(t *testing.T, newAccessToken, newRefreshToken string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/oidc/config":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"token_endpoint": srv.URL + "/token",
			})
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  newAccessToken,
				"refresh_token": newRefreshToken,
				"expires_in":    3600,
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	return srv
}

func TestStoreRefresher_RoundTrip(t *testing.T) {
	srv := setupRefreshServer(t, "new-access", "new-refresh")
	defer srv.Close()

	store := keyringadapter.NewFake()
	const slot = "test-slot"
	_ = store.Put(slot, ports.Tokens{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
	})

	r := &oidcclient.StoreRefresher{
		ServerURL: srv.URL,
		ClientID:  "flow-cli",
		Store:     store,
		Slot:      slot,
	}

	fresh, err := r.RefreshTokens(context.Background())
	if err != nil {
		t.Fatalf("RefreshTokens: %v", err)
	}
	if fresh.AccessToken != "new-access" {
		t.Errorf("AccessToken: got %q, want %q", fresh.AccessToken, "new-access")
	}
	if fresh.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken: got %q, want %q", fresh.RefreshToken, "new-refresh")
	}

	// Verify persisted in store.
	stored, err := store.Get(slot)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if stored.AccessToken != "new-access" {
		t.Errorf("stored AccessToken: got %q, want %q", stored.AccessToken, "new-access")
	}
}

func TestStoreRefresher_NoRefreshToken_ReturnsErrTokenNotFound(t *testing.T) {
	srv := setupRefreshServer(t, "new-access", "new-refresh")
	defer srv.Close()

	store := keyringadapter.NewFake()
	const slot = "test-slot"
	// Store tokens but with no refresh token.
	_ = store.Put(slot, ports.Tokens{AccessToken: "only-access"})

	r := &oidcclient.StoreRefresher{
		ServerURL: srv.URL,
		ClientID:  "flow-cli",
		Store:     store,
		Slot:      slot,
	}

	_, err := r.RefreshTokens(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != ports.ErrTokenNotFound {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestStoreRefresher_TokenURL_Cached(t *testing.T) {
	configCalls := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/oidc/config":
			configCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"token_endpoint": srv.URL + "/token",
			})
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "new-access",
				"refresh_token": "new-refresh",
				"expires_in":    3600,
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	store := keyringadapter.NewFake()
	const slot = "test-slot"
	_ = store.Put(slot, ports.Tokens{AccessToken: "old", RefreshToken: "old-refresh"})

	r := &oidcclient.StoreRefresher{
		ServerURL: srv.URL,
		ClientID:  "flow-cli",
		Store:     store,
		Slot:      slot,
	}

	// First call resolves and caches the token URL.
	if _, err := r.RefreshTokens(context.Background()); err != nil {
		t.Fatalf("first RefreshTokens: %v", err)
	}
	// Refresh the store so a second call has a refresh token to use.
	_ = store.Put(slot, ports.Tokens{AccessToken: "a2", RefreshToken: "r2"})
	if _, err := r.RefreshTokens(context.Background()); err != nil {
		t.Fatalf("second RefreshTokens: %v", err)
	}

	if configCalls != 1 {
		t.Errorf("oidc/config called %d times, want exactly 1 (should be cached)", configCalls)
	}
}
