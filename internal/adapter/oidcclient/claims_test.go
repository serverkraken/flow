package oidcclient

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestUnit_ClaimsFromToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := signTestJWT(t, key, map[string]any{
		"sub": "msoent", "email": "m@x.de", "name": "Soenne",
	})
	c, err := ClaimsFromToken(tok)
	if err != nil {
		t.Fatalf("ClaimsFromToken: %v", err)
	}
	if c.Sub != "msoent" || c.Email != "m@x.de" || c.Name != "Soenne" {
		t.Errorf("got %+v", c)
	}
	if _, err := ClaimsFromToken("not.a.jwt-only-two"); err == nil {
		t.Error("expected error for malformed token")
	}
}

// signTestJWT builds a compact RS256 JWS; signature is irrelevant to the
// decoder but keeps the token well-formed.
func signTestJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	b64 := func(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }
	si := b64(map[string]any{"alg": "RS256", "typ": "JWT"}) + "." + b64(claims)
	sum := sha256.Sum256([]byte(si))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	return si + "." + base64.RawURLEncoding.EncodeToString(sig)
}
