package httpserver

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_AuthBrowser_HandleLogin_RedirectsToIdpWithStateCookie(t *testing.T) {
	t.Parallel()
	ab := newAuthBrowser(testAuthDeps(t))

	rr := httptest.NewRecorder()
	ab.handleLogin(rr, httptest.NewRequest(http.MethodGet, "/login", nil))

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://idp.example/auth") {
		t.Errorf("Location = %q, want prefix https://idp.example/auth", loc)
	}
	if !strings.Contains(loc, "state=") {
		t.Errorf("Location missing state= param: %q", loc)
	}
	if !strings.Contains(loc, "redirect_uri=") {
		t.Errorf("Location missing redirect_uri= param: %q", loc)
	}

	// state cookie present
	var stateCookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "flow_oauth_state" {
			stateCookie = c
		}
	}
	if stateCookie == nil || stateCookie.Value == "" {
		t.Fatal("flow_oauth_state cookie missing or empty")
	}
	if !stateCookie.HttpOnly {
		t.Error("state cookie must be HttpOnly")
	}
}

func TestUnit_AuthBrowser_HandleCallback_StateMismatch_Returns400(t *testing.T) {
	t.Parallel()
	ab := newAuthBrowser(testAuthDeps(t))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=wrong&code=anything", nil)
	req.AddCookie(&http.Cookie{Name: "flow_oauth_state", Value: "expected-state"})
	ab.handleCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestUnit_AuthBrowser_HandleCallback_NoCookie_Returns400(t *testing.T) {
	t.Parallel()
	ab := newAuthBrowser(testAuthDeps(t))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=anything&code=anything", nil)
	ab.handleCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestUnit_AuthBrowser_HandleLogout_ClearsCookieAndRedirects(t *testing.T) {
	t.Parallel()
	ab := newAuthBrowser(testAuthDeps(t))
	rr := httptest.NewRecorder()
	ab.handleLogout(rr, httptest.NewRequest(http.MethodGet, "/logout", nil))

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rr.Code)
	}
	if rr.Header().Get("Location") != "/" {
		t.Errorf("Location = %q, want /", rr.Header().Get("Location"))
	}
	// MaxAge < 0 clears the cookie
	cleared := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "flow_session" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("flow_session cookie must be cleared with MaxAge<0")
	}
}

// --- test helpers ---

type fakeProvider struct {
	id  ports.Identity
	err error
}

func (f fakeProvider) Verify(_ context.Context, _ string) (ports.Identity, error) {
	if f.err != nil {
		return ports.Identity{}, f.err
	}
	return f.id, nil
}

func (fakeProvider) Endpoint() (authURL, tokenURL string) {
	return "https://idp.example/auth", "https://idp.example/token"
}
func (fakeProvider) DeviceAuthorizationURL() string { return "https://idp.example/device" }

type fakeAccess struct{ ok bool }

func (f fakeAccess) Allow(_ ports.Identity) bool { return f.ok }

func testAuthDeps(t *testing.T) AuthDeps {
	t.Helper()
	hashKey, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	blockKey, _ := hex.DecodeString("fedcba9876543210fedcba9876543210")
	sess := NewSession(hashKey, blockKey)
	return AuthDeps{
		Provider:     fakeProvider{id: ports.Identity{Sub: "u-1", Email: "u@x"}},
		Access:       fakeAccess{ok: true},
		Session:      sess,
		BaseURL:      "http://localhost:0",
		OIDCClientID: "test-client",
		OIDCSecret:   "test-secret",
		Cookie:       CookieConfig{Name: "flow_session", Secure: false},
		Ready:        func() error { return nil },
	}
}

// suppress unused-import warning if errors not used in this file:
var _ = errors.New
