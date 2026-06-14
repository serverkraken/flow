package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/clientconfig"
	"github.com/serverkraken/flow/internal/ports"
	"golang.org/x/oauth2"
)

// fakeSource hands out a fixed token.
type fakeSource struct{ tok *oauth2.Token }

func (f fakeSource) Token() (*oauth2.Token, error) { return f.tok, nil }

// memStore is an in-memory TokenStore.
type memStore struct {
	saved ports.Token
	calls int
}

func (m *memStore) Save(t ports.Token) error         { m.saved = t; m.calls++; return nil }
func (m *memStore) Load() (ports.Token, bool, error) { return m.saved, m.calls > 0, nil }
func (m *memStore) Clear() error                     { m.saved = ports.Token{}; return nil }

func TestPersistingSourceSavesOnChange(t *testing.T) {
	store := &memStore{}
	src := &persistingSource{
		base:  fakeSource{tok: &oauth2.Token{AccessToken: "new", RefreshToken: "r", Expiry: time.Unix(10, 0)}},
		store: store,
		last:  ports.Token{AccessToken: "old"},
	}
	if _, err := src.Token(); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || store.saved.AccessToken != "new" {
		t.Fatalf("expected save of new token, got calls=%d saved=%+v", store.calls, store.saved)
	}
	// Second call with an unchanged token must not re-save.
	if _, err := src.Token(); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 {
		t.Fatalf("expected no re-save, calls=%d", store.calls)
	}
}

func TestPersistingSourcePreservesRefreshWhenEmpty(t *testing.T) {
	store := &memStore{}
	src := &persistingSource{
		base:  fakeSource{tok: &oauth2.Token{AccessToken: "a2", RefreshToken: ""}},
		store: store,
		last:  ports.Token{AccessToken: "a1", RefreshToken: "keep"},
	}
	if _, err := src.Token(); err != nil {
		t.Fatal(err)
	}
	if store.saved.RefreshToken != "keep" {
		t.Fatalf("refresh token not preserved: %q", store.saved.RefreshToken)
	}
}

// A still-valid stored token is returned directly, without the OIDC issuer and
// without any network discovery — this is the wart fix that lets `flow whoami`
// work without FLOW_OIDC_ISSUER set.
func TestLazySourceUsesValidTokenWithoutIssuer(t *testing.T) {
	s := &lazyDeviceSource{
		ctx:  context.Background(),
		cfg:  clientconfig.Config{}, // no issuer
		last: ports.Token{AccessToken: "valid", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
	}
	tok, err := s.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "valid" {
		t.Fatalf("expected cached token, got %q", tok.AccessToken)
	}
}

// An expired token with no issuer configured cannot be refreshed; the error
// must point the user at login / the issuer rather than failing obscurely.
func TestLazySourceErrorsWhenExpiredWithoutIssuer(t *testing.T) {
	s := &lazyDeviceSource{
		ctx:  context.Background(),
		cfg:  clientconfig.Config{}, // no issuer
		last: ports.Token{AccessToken: "stale", Expiry: time.Now().Add(-time.Hour)},
	}
	_, err := s.Token()
	if err == nil {
		t.Fatal("expected error for expired token without issuer")
	}
	if !strings.Contains(err.Error(), "FLOW_OIDC_ISSUER") {
		t.Fatalf("error should mention FLOW_OIDC_ISSUER: %v", err)
	}
}

// When the token is expired AND an issuer is configured, the lazy source
// discovers the endpoints and refreshes via the refresh_token grant.
func TestLazySourceRefreshesWhenExpired(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                        srv.URL,
			"authorization_endpoint":        srv.URL + "/auth",
			"token_endpoint":                srv.URL + "/token",
			"device_authorization_endpoint": srv.URL + "/device/code",
			"jwks_uri":                      srv.URL + "/jwks",
		})
	})
	var gotGrant, gotRefresh string
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotGrant = r.Form.Get("grant_type")
		gotRefresh = r.Form.Get("refresh_token")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "fresh-acc",
			"refresh_token": "fresh-ref",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})

	ctx := oidc.InsecureIssuerURLContext(context.Background(), srv.URL)
	s := &lazyDeviceSource{
		ctx:  ctx,
		cfg:  clientconfig.Config{ServerURL: "x", OIDCIssuer: srv.URL, CliClientID: "flow-cli"},
		last: ports.Token{AccessToken: "old-acc", RefreshToken: "old-ref", Expiry: time.Now().Add(-time.Hour)},
	}
	tok, err := s.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "fresh-acc" {
		t.Fatalf("expected refreshed access token, got %q", tok.AccessToken)
	}
	if gotGrant != "refresh_token" || gotRefresh != "old-ref" {
		t.Fatalf("expected refresh_token grant with old refresh token, got grant=%q refresh=%q", gotGrant, gotRefresh)
	}
}
