package oidcserver

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"slices"
	"testing"
	"time"

	oidc "github.com/coreos/go-oidc/v3/oidc"
)

// TestUnit_Verify_MultiIssuer exercises the real multi-verifier loop against two
// issuers with DISTINCT signing keys — the exact situation that produced the
// `flow whoami` 401 (the CLI token's issuer didn't match the single browser
// verifier). No network/Docker: each verifier is built from an in-memory
// oidc.StaticKeySet, and tokens are signed locally with the matching RSA key.
func TestUnit_Verify_MultiIssuer(t *testing.T) {
	t.Parallel()

	keyA := genRSAKey(t)
	keyB := genRSAKey(t)
	const (
		issA = "https://id.example/application/o/flow/"
		issB = "https://id.example/application/o/flow-cli/"
	)

	prov := &Provider{
		verifiers: []*oidc.IDTokenVerifier{
			newStaticVerifier(issA, &keyA.PublicKey),
			newStaticVerifier(issB, &keyB.PublicKey),
		},
		accepted: []string{"flow", "flow-cli"},
	}
	ctx := context.Background()

	// The CLI provider (second issuer, keyB) — this is what regressed.
	tokB := mintRS256(t, keyB, issB, "flow-cli", "msoent", map[string]any{
		"email": "msoent@example.com",
		"name":  "Soenne",
	})
	id, err := prov.Verify(ctx, tokB)
	if err != nil {
		t.Fatalf("Verify(issuer-B/CLI token): %v", err)
	}
	if id.Sub != "msoent" {
		t.Errorf("Sub = %q, want msoent", id.Sub)
	}
	if id.Email != "msoent@example.com" {
		t.Errorf("Email = %q, want msoent@example.com", id.Email)
	}

	// The browser provider (first issuer, keyA) must still validate.
	tokA := mintRS256(t, keyA, issA, "flow", "msoent", nil)
	if _, err := prov.Verify(ctx, tokA); err != nil {
		t.Fatalf("Verify(issuer-A/browser token): %v", err)
	}

	// Correct signature but an issuer outside the trusted set → every verifier
	// rejects it (guards against accidentally trusting any signed token).
	tokUntrusted := mintRS256(t, keyB, "https://id.example/application/o/evil/", "flow-cli", "msoent", nil)
	if _, err := prov.Verify(ctx, tokUntrusted); err == nil {
		t.Fatal("expected rejection: token from untrusted issuer")
	}

	// Trusted issuer claim but signed with the WRONG key → signature must
	// still be enforced (guards against an issuer-only check that skips sig).
	tokForged := mintRS256(t, keyA, issB, "flow-cli", "msoent", nil)
	if _, err := prov.Verify(ctx, tokForged); err == nil {
		t.Fatal("expected rejection: issuer-B claim signed with key A")
	}

	// Valid issuer + signature but an audience we don't accept → the audience
	// gate (post-success) rejects it.
	tokBadAud := mintRS256(t, keyB, issB, "some-other-app", "msoent", nil)
	if _, err := prov.Verify(ctx, tokBadAud); err == nil {
		t.Fatal("expected rejection: audience not in accepted set")
	}
}

func TestUnit_dedupeIssuers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, []string{}},
		{"single", []string{"a"}, []string{"a"}},
		{"identical pair collapses (dex stack)", []string{"a", "a"}, []string{"a"}},
		{"two distinct preserved (authentik)", []string{"a", "b"}, []string{"a", "b"}},
		{"drops empty strings", []string{"", "a", "", "b", ""}, []string{"a", "b"}},
		{"order preserved, later dupes dropped", []string{"b", "a", "b"}, []string{"b", "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := dedupeIssuers(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("dedupeIssuers(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestUnit_NewProvider_RejectsEmptyIssuers(t *testing.T) {
	t.Parallel()
	// AcceptedClientIDs is non-empty so we get past the first guard and hit the
	// Issuers check — which must reject before any network discovery runs.
	_, err := NewProvider(t.Context(), ProviderConfig{
		Issuers:           nil,
		AcceptedClientIDs: []string{"flow"},
	})
	if err == nil {
		t.Fatal("NewProvider with empty Issuers: expected error, got nil")
	}
}

// newStaticVerifier builds an oidc verifier bound to one issuer and one public
// key, with the same audience policy NewProvider uses (skip the built-in
// single-aud check; Verify enforces the accepted-list).
func newStaticVerifier(issuer string, pub *rsa.PublicKey) *oidc.IDTokenVerifier {
	return oidc.NewVerifier(
		issuer,
		&oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{pub}},
		&oidc.Config{ClientID: "unused", SkipClientIDCheck: true},
	)
}

func genRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	return k
}

// mintRS256 hand-rolls a compact RS256 JWS (header.payload.signature) so the
// test depends only on stdlib crypto — no JWT library. go-oidc's StaticKeySet
// parses and verifies it like any real token.
func mintRS256(t *testing.T, key *rsa.PrivateKey, iss, aud, sub string, extra map[string]any) string {
	t.Helper()
	now := time.Now()
	claims := map[string]any{
		"iss": iss,
		"aud": aud,
		"sub": sub,
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}
	signingInput := b64urlJSON(t, map[string]any{"alg": "RS256", "typ": "JWT"}) +
		"." + b64urlJSON(t, claims)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func b64urlJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
