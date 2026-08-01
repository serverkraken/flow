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
	return mintRS256Auds(t, key, iss, aud)
}

// mintRS256Auds is mintRS256 with a MULTI-VALUED `aud`. `aud` is legitimately
// an array in JWT, and the array form is what exposes order-dependent audience
// matching — a single-audience helper cannot reach that bug at all.
func mintRS256Auds(t *testing.T, key *rsa.PrivateKey, iss string, auds ...string) string {
	t.Helper()
	// A one-element []string marshals to a JSON array too, which go-oidc parses
	// the same as the scalar form — so the helper stays honest for both shapes.
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":                iss,
		"aud":                auds,
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
func staticIssuerVerifier(iss string, pub *rsa.PublicKey, auds ...Audience) issuerVerifier {
	allow := map[string]bool{}
	for _, a := range auds {
		allow[a.ClientID] = a.Machine
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
		staticIssuerVerifier(issA, &keyA.PublicKey, Audience{ClientID: "flow"}),
		staticIssuerVerifier(issB, &keyB.PublicKey, Audience{ClientID: "flow-cli"}),
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

// TestVerifyMachineFlagIsOrderIndependent pins that a multi-valued `aud` is
// classified by CONTENT, not by position. Before the fix, Verify broke on the
// first audience found in the issuer's map, so aud=[flow-machine flow-dev] was
// Machine=true while the same two swapped was Machine=false — and human is the
// LESS restricted verdict (own user row, own tenant, every s.auth route),
// so the order-dependent path was a privilege escalation.
func TestVerifyMachineFlagIsOrderIndependent(t *testing.T) {
	key := genKey(t)
	const iss = "http://localhost:5556/dex"
	vr := &Verifier{verifiers: []issuerVerifier{
		staticIssuerVerifier(iss, &key.PublicKey,
			Audience{ClientID: "flow-dev"},
			Audience{ClientID: "flow-cli"},
			Audience{ClientID: "flow-machine", Machine: true}),
	}}
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		auds []string
		want bool
	}{
		{"machine audience first", []string{"flow-machine", "flow-dev"}, true},
		{"machine audience last", []string{"flow-dev", "flow-machine"}, true},
		{"machine audience in the middle", []string{"flow-dev", "flow-machine", "flow-cli"}, true},
		{"no machine audience at all", []string{"flow-dev", "flow-cli"}, false},
		// An unknown audience alongside a known one must not change the verdict:
		// unknown entries are skipped, not treated as a miss.
		{"unknown audience before the machine one", []string{"evil", "flow-machine"}, true},
		{"unknown audience before a human one", []string{"evil", "flow-dev"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, err := vr.Verify(ctx, mintRS256Auds(t, key, iss, tc.auds...))
			if err != nil {
				t.Fatalf("aud %v: %v", tc.auds, err)
			}
			if id.Machine != tc.want {
				t.Fatalf("aud %v: Machine = %v, want %v", tc.auds, id.Machine, tc.want)
			}
		})
	}

	// No audience matches at all → still a hard reject, unchanged.
	if _, err := vr.Verify(ctx, mintRS256Auds(t, key, iss, "evil", "also-evil")); err == nil {
		t.Fatal("expected reject: no known audience")
	}
}

func TestNewRejectsEmptyPairs(t *testing.T) {
	if _, err := New(context.Background(), nil); err == nil {
		t.Fatal("expected error for empty pairs")
	}
}

func TestNewRejectsEmptyIssuer(t *testing.T) {
	if _, err := New(context.Background(), []IssuerAudiences{{Issuer: "", Audiences: []Audience{{ClientID: "x"}}}}); err == nil {
		t.Fatal("expected error for empty issuer")
	}
}

func TestNewRejectsEmptyAudiences(t *testing.T) {
	if _, err := New(context.Background(), []IssuerAudiences{{Issuer: "https://issuer.example", Audiences: nil}}); err == nil {
		t.Fatal("expected error for empty audiences")
	}
}

func TestVerifyStampsMachineFromAudience(t *testing.T) {
	keyWeb, keyMachine := genKey(t), genKey(t)
	const (
		issWeb     = "https://id.example/application/o/flow/"
		issMachine = "https://id.example/application/o/flow-machine/"
	)
	vr := &Verifier{verifiers: []issuerVerifier{
		staticIssuerVerifier(issWeb, &keyWeb.PublicKey, Audience{ClientID: "flow"}),
		staticIssuerVerifier(issMachine, &keyMachine.PublicKey,
			Audience{ClientID: "flow-machine", Machine: true}),
	}}
	ctx := context.Background()

	// A machine token is stamped.
	id, err := vr.Verify(ctx, mintRS256(t, keyMachine, issMachine, "flow-machine"))
	if err != nil {
		t.Fatalf("machine token: %v", err)
	}
	if !id.Machine {
		t.Fatal("machine token must set Identity.Machine")
	}

	// A human token is not.
	id, err = vr.Verify(ctx, mintRS256(t, keyWeb, issWeb, "flow"))
	if err != nil {
		t.Fatalf("web token: %v", err)
	}
	if id.Machine {
		t.Fatal("web token must not set Identity.Machine")
	}

	// The machine audience presented on the WEB issuer is rejected outright —
	// it must never be a route to Machine=true on a human-issued token.
	if _, err := vr.Verify(ctx, mintRS256(t, keyWeb, issWeb, "flow-machine")); err == nil {
		t.Fatal("expected reject: machine audience on web issuer")
	}
}

func TestVerifyDevSingleIssuerStampsOnlyTheMachineAudience(t *testing.T) {
	key := genKey(t)
	const iss = "http://localhost:5556/dex"
	vr := &Verifier{verifiers: []issuerVerifier{
		staticIssuerVerifier(iss, &key.PublicKey,
			Audience{ClientID: "flow-dev"},
			Audience{ClientID: "flow-cli"},
			Audience{ClientID: "flow-machine", Machine: true}),
	}}
	ctx := context.Background()

	for _, tc := range []struct {
		aud  string
		want bool
	}{
		{"flow-dev", false},
		{"flow-cli", false},
		{"flow-machine", true},
	} {
		id, err := vr.Verify(ctx, mintRS256(t, key, iss, tc.aud))
		if err != nil {
			t.Fatalf("aud %s: %v", tc.aud, err)
		}
		if id.Machine != tc.want {
			t.Fatalf("aud %s: Machine = %v, want %v", tc.aud, id.Machine, tc.want)
		}
	}
}
