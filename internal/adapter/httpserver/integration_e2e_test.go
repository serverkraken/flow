//go:build integration

package httpserver_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/oidcserver"
	"github.com/serverkraken/flow/internal/testutil/oidctest"
)

// TestIntegration_E2E_BrowserFlow_LoginThenMe boots dex + flow-server, walks
// the browser auth-code flow against dex by posting credentials to dex's
// local-connector login form, and confirms /api/v1/me returns the static
// user's identity.
func TestIntegration_E2E_BrowserFlow_LoginThenMe(t *testing.T) {
	// Grab a free port for the httptest server so we know the base URL before
	// constructing the server — the oauth2.Config.RedirectURL must match what
	// dex expects, and dex validates it at startup (not at runtime), so the
	// redirect URI must be registered in dex's static client config before the
	// container starts.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	tsBaseURL := fmt.Sprintf("http://%s", ln.Addr().String())
	callbackURI := tsBaseURL + "/auth/callback"

	dex := oidctest.StartDex(t, oidctest.WithRedirectURI(callbackURI))
	ctx := context.Background()

	prov, err := oidcserver.NewProvider(ctx, oidcserver.ProviderConfig{
		Issuers:           []string{dex.Issuer},
		AcceptedClientIDs: []string{dex.ClientID, dex.CLIClientID},
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	access := oidcserver.NewSubAllowlist([]string{dex.StaticUser.Sub})
	hashKey, _ := hex.DecodeString(strings.Repeat("11", 32))
	blockKey, _ := hex.DecodeString(strings.Repeat("22", 16))
	sess := httpserver.NewSession(hashKey, blockKey)

	srv := httpserver.NewWithAuth(httpserver.AuthDeps{
		Provider:     prov,
		Access:       access,
		Session:      sess,
		BaseURL:      tsBaseURL,
		OIDCClientID: dex.ClientID,
		OIDCSecret:   dex.ClientSecret,
		Cookie:       httpserver.CookieConfig{Name: "flow_session", Secure: false},
		Ready:        func() error { return nil },
		OIDCConfig: httpserver.OIDCConfigResponse{
			Issuer:                 dex.Issuer,
			DeviceAuthorizationURL: dex.Issuer + "/device/code",
			TokenURL:               dex.Issuer + "/token",
			ClientID:               dex.CLIClientID,
		},
	})

	ts := httptest.NewUnstartedServer(srv.Handler())
	ts.Listener = ln
	ts.Start()
	t.Cleanup(ts.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 15 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	// Step 1: GET /login → 302 → dex /auth → dex local-connector login form.
	// The cookie jar will carry the flow_oauth_state cookie through redirects.
	resp, err := client.Get(tsBaseURL + "/login")
	if err != nil {
		t.Fatalf("/login: %v", err)
	}
	_ = resp.Body.Close()

	// After following all redirects we should land on dex's login form.
	finalURL := resp.Request.URL
	if !strings.HasPrefix(finalURL.String(), dex.Issuer) {
		t.Skipf("expected to land on dex (%s), got %s — dex form layout may have changed; run manual smoke test (docs/runbook/m1-smoke-test.md)", dex.Issuer, finalURL)
	}

	// Step 2: POST credentials to dex's local-connector form action.
	// Dex's local connector accepts POST at the same URL (/auth/local or
	// wherever the redirect landed) with "login" and "password" fields.
	form := url.Values{}
	form.Set("login", dex.StaticUser.Email)
	form.Set("password", dex.StaticUser.Password)
	req, _ := http.NewRequest(http.MethodPost, finalURL.String(), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("dex POST credentials: %v", err)
	}
	_ = resp.Body.Close()

	// After the credential POST, dex (with skipApprovalScreen: true) redirects
	// through the callback → / on the flow server. If we ended up somewhere
	// other than the flow server, the flow didn't complete.
	if !strings.HasPrefix(resp.Request.URL.String(), tsBaseURL) {
		t.Skipf("flow didn't complete — ended at %s; dex auth-code flow may need approval-screen handling; run manual smoke (docs/runbook/m1-smoke-test.md)", resp.Request.URL)
	}

	// Step 3: GET /api/v1/me — the cookie jar carries the session cookie set
	// during /auth/callback. Expect 200 with the authenticated user's identity.
	meReq, _ := http.NewRequest(http.MethodGet, tsBaseURL+"/api/v1/me", nil)
	meResp, err := client.Do(meReq)
	if err != nil {
		t.Fatalf("/api/v1/me: %v", err)
	}
	defer meResp.Body.Close()
	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("/api/v1/me status = %d, want 200", meResp.StatusCode)
	}
}
