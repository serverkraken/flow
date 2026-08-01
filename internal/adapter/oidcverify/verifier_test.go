package oidcverify_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/serverkraken/flow/internal/adapter/oidcverify"
)

// oidcHarness holds the shared test server, RSA key, and issuer URL used
// across multiple verifier tests.
type oidcHarness struct {
	key    *rsa.PrivateKey
	kid    string
	issuer string
	srv    *httptest.Server
}

// newOIDCHarness starts an httptest JWKS+discovery server and returns a harness.
// Callers must defer h.srv.Close().
func newOIDCHarness(t *testing.T) *oidcHarness {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-key"
	h := &oidcHarness{key: key, kid: kid}

	mux := http.NewServeMux()
	h.srv = httptest.NewServer(mux)
	h.issuer = h.srv.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                h.issuer,
			"jwks_uri":                              h.issuer + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{
			{"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig", "n": n, "e": e},
		}})
	})
	return h
}

// signToken creates and signs a JWT with the harness key, merging the provided
// extra claims on top of a set of valid baseline claims.
func (h *oidcHarness) signToken(t *testing.T, overrides jwt.MapClaims) string {
	t.Helper()
	base := jwt.MapClaims{
		"iss":                h.issuer,
		"aud":                "flow",
		"sub":                "msoent",
		"preferred_username": "msoent",
		"email":              "m@x.de",
		"name":               "Martin",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
	}
	for k, v := range overrides {
		base[k] = v
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, base)
	tok.Header["kid"] = h.kid
	signed, err := tok.SignedString(h.key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestVerifyValidToken(t *testing.T) {
	h := newOIDCHarness(t)
	defer h.srv.Close()

	signed := h.signToken(t, nil)

	ctx := oidc.InsecureIssuerURLContext(context.Background(), h.issuer)
	v, err := oidcverify.New(ctx, []oidcverify.IssuerAudiences{{Issuer: h.issuer, Audiences: []oidcverify.Audience{{ClientID: "flow"}}}})
	if err != nil {
		t.Fatal(err)
	}
	id, err := v.Verify(context.Background(), signed)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if id.Subject != "msoent" || id.Username != "msoent" {
		t.Fatalf("identity mismatch: %+v", id)
	}

	// Garbage token must be rejected by the same verifier.
	if _, err := v.Verify(context.Background(), "not.a.valid.jwt"); err == nil {
		t.Fatal("expected error for garbage token")
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	h := newOIDCHarness(t)
	defer h.srv.Close()

	signedExpired := h.signToken(t, jwt.MapClaims{
		"exp": time.Now().Add(-time.Hour).Unix(),
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
	})

	ctx := oidc.InsecureIssuerURLContext(context.Background(), h.issuer)
	v, err := oidcverify.New(ctx, []oidcverify.IssuerAudiences{{Issuer: h.issuer, Audiences: []oidcverify.Audience{{ClientID: "flow"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), signedExpired); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestVerifyRejectsWrongAudience(t *testing.T) {
	h := newOIDCHarness(t)
	defer h.srv.Close()

	signedWrongAud := h.signToken(t, jwt.MapClaims{
		"aud": "some-other-client",
	})

	ctx := oidc.InsecureIssuerURLContext(context.Background(), h.issuer)
	v, err := oidcverify.New(ctx, []oidcverify.IssuerAudiences{{Issuer: h.issuer, Audiences: []oidcverify.Audience{{ClientID: "flow"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), signedWrongAud); err == nil {
		t.Fatal("expected error for wrong audience")
	}
}

func TestVerifyAcceptsSecondAudience(t *testing.T) {
	h := newOIDCHarness(t)
	defer h.srv.Close()

	// A token whose aud is the CLI client, not the primary web client.
	cliToken := h.signToken(t, jwt.MapClaims{"aud": "flow-cli"})

	ctx := oidc.InsecureIssuerURLContext(context.Background(), h.issuer)
	v, err := oidcverify.New(ctx, []oidcverify.IssuerAudiences{{Issuer: h.issuer, Audiences: []oidcverify.Audience{{ClientID: "flow"}, {ClientID: "flow-cli"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), cliToken); err != nil {
		t.Fatalf("verify CLI-audience token: %v", err)
	}

	// An audience outside the allowed set is still rejected.
	other := h.signToken(t, jwt.MapClaims{"aud": "evil"})
	if _, err := v.Verify(context.Background(), other); err == nil {
		t.Fatal("expected rejection for audience outside the allowed set")
	}
}
