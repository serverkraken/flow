package oidcauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

func TestExchangeReturnsIdentity(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-key"

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	issuer := srv.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 issuer,
			"authorization_endpoint": issuer + "/authorize",
			"token_endpoint":         issuer + "/token",
			"jwks_uri":               issuer + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{
			{"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig", "n": n, "e": e},
		}})
	})

	// Build and sign an id_token.
	claims := jwt.MapClaims{
		"iss":                issuer,
		"aud":                "flow",
		"sub":                "msoent",
		"preferred_username": "msoent",
		"email":              "m@x.de",
		"name":               "Martin",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
	}
	jwtTok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	jwtTok.Header["kid"] = kid
	idToken, err := jwtTok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "acc-tok",
			"token_type":   "bearer",
			"id_token":     idToken,
			"expires_in":   3600,
		})
	})

	ctx := oidc.InsecureIssuerURLContext(context.Background(), issuer)
	a, err := New(ctx, issuer, "flow", "secret", issuer+"/callback")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	id, err := a.Exchange(ctx, "auth-code-xyz")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if id.Subject != "msoent" || id.Username != "msoent" || id.Email != "m@x.de" {
		t.Fatalf("identity mismatch: %+v", id)
	}
}

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
