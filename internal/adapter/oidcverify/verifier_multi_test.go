package oidcverify

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

func genKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// mintRS256 signs a minimal token with the given key/issuer/audience.
func mintRS256(t *testing.T, key *rsa.PrivateKey, iss, aud string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":                iss,
		"aud":                aud,
		"sub":                "msoent",
		"preferred_username": "msoent",
		"email":              "m@x.de",
		"name":               "Martin",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
	})
	tok.Header["kid"] = "k"
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// staticIssuerVerifier builds a network-free issuerVerifier bound to one issuer,
// one key, and one allowed audience set.
func staticIssuerVerifier(iss string, pub *rsa.PublicKey, auds ...string) issuerVerifier {
	allow := map[string]bool{}
	for _, a := range auds {
		allow[a] = true
	}
	return issuerVerifier{
		v: oidc.NewVerifier(iss,
			&oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{pub}},
			&oidc.Config{SkipClientIDCheck: true}),
		auds: allow,
	}
}

func TestVerifyMultiIssuerPerIssuerAudience(t *testing.T) {
	keyA, keyB := genKey(t), genKey(t)
	const (
		issA = "https://id.example/application/o/flow/"
		issB = "https://id.example/application/o/flow-cli/"
	)
	vr := &Verifier{verifiers: []issuerVerifier{
		staticIssuerVerifier(issA, &keyA.PublicKey, "flow"),
		staticIssuerVerifier(issB, &keyB.PublicKey, "flow-cli"),
	}}
	ctx := context.Background()

	// Web token on the web issuer → accepted, identity mapped.
	id, err := vr.Verify(ctx, mintRS256(t, keyA, issA, "flow"))
	if err != nil {
		t.Fatalf("web token: %v", err)
	}
	if id.Subject != "msoent" || id.Username != "msoent" {
		t.Fatalf("identity mismatch: %+v", id)
	}

	// CLI token on the CLI issuer (the path that regressed) → accepted.
	if _, err := vr.Verify(ctx, mintRS256(t, keyB, issB, "flow-cli")); err != nil {
		t.Fatalf("cli token: %v", err)
	}

	// Per-issuer strictness: CLI audience presented on the WEB issuer → reject.
	if _, err := vr.Verify(ctx, mintRS256(t, keyA, issA, "flow-cli")); err == nil {
		t.Fatal("expected reject: cli audience on web issuer")
	}

	// Per-issuer strictness: web audience on the CLI issuer → reject.
	if _, err := vr.Verify(ctx, mintRS256(t, keyB, issB, "flow")); err == nil {
		t.Fatal("expected reject: web audience on cli issuer")
	}

	// Forged: issB claim signed with key A → signature reject (defence-in-depth).
	if _, err := vr.Verify(ctx, mintRS256(t, keyA, issB, "flow-cli")); err == nil {
		t.Fatal("expected reject: issB claim signed with key A")
	}

	// Untrusted issuer → reject.
	if _, err := vr.Verify(ctx, mintRS256(t, keyB, "https://id.example/application/o/evil/", "flow-cli")); err == nil {
		t.Fatal("expected reject: untrusted issuer")
	}
}

func TestNewRejectsEmptyPairs(t *testing.T) {
	if _, err := New(context.Background(), nil); err == nil {
		t.Fatal("expected error for empty pairs")
	}
}
