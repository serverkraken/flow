package httpapi_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/ports"
)

// TestClient_ValidToken_MeBearer200 verifies that a minted bearer token is
// accepted by GET /api/v1/me-bearer and returns 200.
func TestClient_ValidToken_MeBearer200(t *testing.T) {
	api := newTestAPI(t)

	// Mint a fresh token and call /api/v1/me-bearer directly.
	idToken := api.MintToken(api.Sub)
	req, err := http.NewRequest(http.MethodGet, api.URL+"/api/v1/me-bearer", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+idToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /me-bearer: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestClient_EmptyTokenStore_ErrLoggedOut verifies that a client with no
// stored token returns ErrLoggedOut when calling a resource method.
func TestClient_EmptyTokenStore_ErrLoggedOut(t *testing.T) {
	api := newTestAPI(t)
	cli := httpapi.New(httpapi.Config{
		BaseURL: api.URL,
		Tokens:  &memTokens{ok: false},
		Slot:    "test",
	})
	sessions := httpapi.NewSessions(cli)
	_, err := sessions.Load("")
	if !errors.Is(err, httpapi.ErrLoggedOut) {
		t.Errorf("got %v, want ErrLoggedOut", err)
	}
}

// TestClient_EmptyBaseURL_ErrNotConfigured verifies that a client with no
// BaseURL returns ErrNotConfigured when calling a resource method.
func TestClient_EmptyBaseURL_ErrNotConfigured(t *testing.T) {
	cli := httpapi.New(httpapi.Config{
		BaseURL: "",
		Tokens:  &memTokens{ok: true, tok: ports.Tokens{AccessToken: "tok"}},
		Slot:    "test",
	})
	sessions := httpapi.NewSessions(cli)
	_, err := sessions.Load("")
	if !errors.Is(err, httpapi.ErrNotConfigured) {
		t.Errorf("got %v, want ErrNotConfigured", err)
	}
}

// TestClient_ServerStopped_ErrUnavailable verifies that a stopped server
// returns ErrUnavailable when the cache is empty.
func TestClient_ServerStopped_ErrUnavailable(t *testing.T) {
	api := newTestAPI(t)
	// Use a fresh client without the shared snapshot path interfering
	cli := httpapi.New(httpapi.Config{
		BaseURL: api.URL,
		Tokens:  &memTokens{ok: true, tok: ports.Tokens{AccessToken: "stopped-tok"}},
		Slot:    "test-stopped",
	})
	sessions := httpapi.NewSessions(cli)
	api.server.Close() // stop the server before any successful read
	_, err := sessions.Load("")
	if !errors.Is(err, httpapi.ErrUnavailable) {
		t.Errorf("got %v, want ErrUnavailable", err)
	}
}

// Compile-time interface assertions.
var (
	_ ports.TokenStore = (*memTokens)(nil)
	_ *httpapi.Client  = (*httpapi.Client)(nil)
)
