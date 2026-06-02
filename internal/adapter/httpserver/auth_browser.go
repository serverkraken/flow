package httpserver

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/ports"
	"golang.org/x/oauth2"
)

// CookieConfig holds runtime options for the session cookie. Secure must be
// true in production but false for HTTP-only local dev (Docker, no TLS).
type CookieConfig struct {
	Name   string
	Secure bool
}

// oidcserverProvider is the subset of *oidcserver.Provider that this package
// needs. Declared as a local interface to avoid importing the oidcserver
// adapter (and risking import cycles when oidcserver eventually grows).
//
// *oidcserver.Provider satisfies this interface by construction.
type oidcserverProvider interface {
	ports.AuthProvider
	Endpoint() (authURL, tokenURL string)
	DeviceAuthorizationURL() string
}

// AuthDeps bundles all dependencies needed by the auth-code handlers.
// Construction lives in cmd/flow-server/main.go.
//
// OIDCConfig is populated even in Task 10 (used by /api/v1/oidc/config in
// Task 17). It's safe to leave the zero-value if no one calls that endpoint.
type AuthDeps struct {
	Provider     oidcserverProvider
	Access       ports.AccessChecker
	Session      ports.BrowserSessionStore
	BaseURL      string
	OIDCClientID string
	OIDCSecret   string
	Cookie       CookieConfig
	Ready        ReadinessCheck
	OIDCConfig   OIDCConfigResponse // populated by main; consumed by Task 17 handler
}

// authBrowser holds the OAuth2 config + deps for the three browser endpoints.
type authBrowser struct {
	deps AuthDeps
	oa   oauth2.Config
}

func newAuthBrowser(d AuthDeps) *authBrowser {
	authURL, tokenURL := d.Provider.Endpoint()
	return &authBrowser{
		deps: d,
		oa: oauth2.Config{
			ClientID:     d.OIDCClientID,
			ClientSecret: d.OIDCSecret,
			RedirectURL:  d.BaseURL + "/auth/callback",
			Endpoint:     oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL},
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile", "offline_access"},
		},
	}
}

// sessionValue is the encrypted payload of the session cookie. Tiny on
// purpose — anything bigger goes in a server-side store later. Stored
// independently of ports.Identity to keep cookie format simple.
type sessionValue struct {
	Sub       string
	Email     string
	Name      string
	ExpiresAt int64
}

const stateCookieName = "flow_oauth_state"

func (ab *authBrowser) handleLogin(w http.ResponseWriter, r *http.Request) {
	state := randomState()
	// Persist state in a short-lived cookie so /auth/callback can verify
	// it. CSRF defence.
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   ab.deps.Cookie.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, ab.oa.AuthCodeURL(state), http.StatusFound)
}

func (ab *authBrowser) handleCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}
	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   ab.deps.Cookie.Secure,
		SameSite: http.SameSiteLaxMode,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "no code", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	tok, err := ab.oa.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "token exchange: "+err.Error(), http.StatusBadGateway)
		return
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		http.Error(w, "no id_token in response", http.StatusBadGateway)
		return
	}
	id, err := ab.deps.Provider.Verify(ctx, rawID)
	if err != nil {
		http.Error(w, "id_token verify: "+err.Error(), http.StatusUnauthorized)
		return
	}
	if !ab.deps.Access.Allow(id) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	enc, err := ab.deps.Session.Encode(ab.deps.Cookie.Name, sessionValue{
		Sub:       id.Sub,
		Email:     id.Email,
		Name:      id.Name,
		ExpiresAt: id.ExpiresAt.Unix(),
	})
	if err != nil {
		http.Error(w, "session encode", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     ab.deps.Cookie.Name,
		Value:    enc,
		Path:     "/",
		Expires:  time.Now().Add(8 * time.Hour),
		HttpOnly: true,
		Secure:   ab.deps.Cookie.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (ab *authBrowser) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     ab.deps.Cookie.Name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   ab.deps.Cookie.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func randomState() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
