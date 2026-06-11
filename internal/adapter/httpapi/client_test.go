package httpapi_test

import (
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestClient_EmptyTokenStore_ErrLoggedOut verifies that a client with no
// stored token returns ErrLoggedOut. Requires an exported method (Task 3).
func TestClient_EmptyTokenStore_ErrLoggedOut(t *testing.T) {
	t.Skip("requires exported method from Task 3 — placeholder")
	// When Task 3 lands, replace with:
	//   cli := httpapi.New(httpapi.Config{
	//       BaseURL: someURL,
	//       Tokens:  &memTokens{ok: false},
	//       Slot:    "test",
	//   })
	//   _, err := cli.ListProjects(context.Background())
	//   if !errors.Is(err, httpapi.ErrLoggedOut) {
	//       t.Errorf("got %v, want ErrLoggedOut", err)
	//   }
}

// TestClient_EmptyBaseURL_ErrNotConfigured verifies that a client with no
// BaseURL returns ErrNotConfigured. Requires an exported method (Task 3).
func TestClient_EmptyBaseURL_ErrNotConfigured(t *testing.T) {
	t.Skip("requires exported method from Task 3 — placeholder")
	// When Task 3 lands, replace with:
	//   cli := httpapi.New(httpapi.Config{
	//       BaseURL: "",
	//       Tokens:  &memTokens{ok: true, tok: ports.Tokens{AccessToken: "tok"}},
	//       Slot:    "test",
	//   })
	//   _, err := cli.ListProjects(context.Background())
	//   if !errors.Is(err, httpapi.ErrNotConfigured) {
	//       t.Errorf("got %v, want ErrNotConfigured", err)
	//   }
}

// TestClient_ServerStopped_ErrUnavailable verifies that a stopped server
// returns ErrUnavailable. Requires an exported method (Task 3).
func TestClient_ServerStopped_ErrUnavailable(t *testing.T) {
	t.Skip("requires exported method from Task 3 — placeholder")
	// When Task 3 lands, replace with:
	//   api := newTestAPI(t)
	//   api.Server.Close() // stop the server
	//   _, err := api.Client.ListProjects(context.Background())
	//   if !errors.Is(err, httpapi.ErrUnavailable) {
	//       t.Errorf("got %v, want ErrUnavailable", err)
	//   }
}

// Compile-time interface assertions.
var (
	_ ports.TokenStore = (*memTokens)(nil)
	_ *httpapi.Client  = (*httpapi.Client)(nil)
)
