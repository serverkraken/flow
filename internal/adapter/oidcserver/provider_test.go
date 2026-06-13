//go:build integration

package oidcserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/oidcserver"
	"github.com/serverkraken/flow/internal/testutil/oidctest"
)

func TestIntegration_Provider_VerifyValidIDToken(t *testing.T) {
	dex := oidctest.StartDex(t)
	ctx := context.Background()

	prov, err := oidcserver.NewProvider(ctx, oidcserver.ProviderConfig{
		Issuers:           []string{dex.Issuer},
		AcceptedClientIDs: []string{dex.ClientID},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	idToken := mintIDTokenViaROPC(t, dex)

	id, err := prov.Verify(ctx, idToken)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.Sub != dex.StaticUser.Sub {
		t.Errorf("Sub = %q, want %q", id.Sub, dex.StaticUser.Sub)
	}
	if id.Email != dex.StaticUser.Email {
		t.Errorf("Email = %q, want %q", id.Email, dex.StaticUser.Email)
	}
}

func TestIntegration_Provider_RejectsTamperedToken(t *testing.T) {
	dex := oidctest.StartDex(t)
	ctx := context.Background()
	prov, err := oidcserver.NewProvider(ctx, oidcserver.ProviderConfig{
		Issuers:           []string{dex.Issuer},
		AcceptedClientIDs: []string{dex.ClientID},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	tampered := "header.payload.signature"
	if _, err := prov.Verify(ctx, tampered); err == nil {
		t.Fatal("expected verify error on bogus token")
	}
}

// TestIntegration_Provider_VerifyAcceptsSecondIssuer boots TWO dex instances —
// two distinct issuer URLs AND two distinct signing keys, mirroring Authentik's
// per_provider split into flow-web + flow-cli. It proves the multi-verifier
// accepts a token from the second issuer (the live `flow whoami` 401 was a
// single browser-issuer verifier rejecting the CLI token) and that a
// single-issuer provider still rejects it.
func TestIntegration_Provider_VerifyAcceptsSecondIssuer(t *testing.T) {
	dex1 := oidctest.StartDex(t)
	dex2 := oidctest.StartDex(t)
	ctx := context.Background()

	multi, err := oidcserver.NewProvider(ctx, oidcserver.ProviderConfig{
		Issuers:           []string{dex1.Issuer, dex2.Issuer},
		AcceptedClientIDs: []string{dex1.ClientID},
	})
	if err != nil {
		t.Fatalf("NewProvider(multi): %v", err)
	}

	// Token minted by the SECOND issuer must validate against its own verifier.
	tok2 := mintIDTokenViaROPC(t, dex2)
	id, err := multi.Verify(ctx, tok2)
	if err != nil {
		t.Fatalf("Verify(second-issuer token): %v", err)
	}
	if id.Sub != dex2.StaticUser.Sub {
		t.Errorf("Sub = %q, want %q", id.Sub, dex2.StaticUser.Sub)
	}

	// Regression guard: trusting ONLY the first issuer must reject the second
	// issuer's token — this is precisely the bug being fixed.
	single, err := oidcserver.NewProvider(ctx, oidcserver.ProviderConfig{
		Issuers:           []string{dex1.Issuer},
		AcceptedClientIDs: []string{dex1.ClientID},
	})
	if err != nil {
		t.Fatalf("NewProvider(single): %v", err)
	}
	if _, err := single.Verify(ctx, tok2); err == nil {
		t.Fatal("single-issuer provider must reject a token from the other issuer")
	}
}

// mintIDTokenViaROPC drives dex's Resource-Owner-Password-Credentials grant
// (enabled via enablePasswordDB in our dex config) to mint a valid ID token
// for the static user without a browser. dex 2.41.1 still ships ROPC for the
// passwordDB connector — this gives us a hex path that avoids parsing HTML.
func mintIDTokenViaROPC(t *testing.T, dex *oidctest.Instance) string {
	t.Helper()
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("username", dex.StaticUser.Email)
	form.Set("password", dex.StaticUser.Password)
	form.Set("scope", "openid email profile")

	req, _ := http.NewRequest(http.MethodPost, dex.Issuer+"/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(dex.ClientID, dex.ClientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("ROPC token request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		t.Skipf("ROPC unavailable on this dex (status %d, body %s) — skipping; cover via /login flow in Task 10", resp.StatusCode, string(body))
	}
	var raw struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if raw.IDToken == "" {
		t.Skip("token response had no id_token; cover via /login flow in Task 10")
	}
	return raw.IDToken
}
