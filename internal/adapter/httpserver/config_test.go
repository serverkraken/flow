package httpserver

import (
	"slices"
	"testing"
)

func TestUnit_LoadConfig_Defaults_WhenEnvUnset(t *testing.T) {
	// Truly unset all FLOW_ env vars so defaults kick in.
	envs := []string{
		"FLOW_SERVER_ADDR", "FLOW_SERVER_BASE_URL",
		"FLOW_OIDC_ISSUER", "FLOW_OIDC_CLIENT_ID", "FLOW_OIDC_CLIENT_SECRET",
		"FLOW_COOKIE_HASH_KEY", "FLOW_COOKIE_BLOCK_KEY", "FLOW_ALLOWED_SUBS",
	}
	for _, e := range envs {
		t.Setenv(e, "")
	}

	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Addr != ":8080" {
		t.Errorf("Addr default = %q, want %q", got.Addr, ":8080")
	}
	if got.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL default = %q, want %q", got.BaseURL, "http://localhost:8080")
	}
	if got.OIDCIssuer != "" || got.OIDCClientID != "" || got.OIDCClientSecret != "" {
		t.Errorf("OIDC fields should be empty when unset; got %+v", got)
	}
	if got.AllowedSubs != nil {
		t.Errorf("AllowedSubs should be nil when unset; got %v", got.AllowedSubs)
	}
}

func TestUnit_LoadConfig_AllOverridden(t *testing.T) {
	t.Setenv("FLOW_SERVER_ADDR", ":9000")
	t.Setenv("FLOW_SERVER_BASE_URL", "https://flow.example.com")
	t.Setenv("FLOW_OIDC_ISSUER", "https://auth.example.com/realms/flow")
	t.Setenv("FLOW_OIDC_CLIENT_ID", "flow-server")
	t.Setenv("FLOW_OIDC_CLIENT_SECRET", "secret")
	t.Setenv("FLOW_COOKIE_HASH_KEY", "abcd")
	t.Setenv("FLOW_COOKIE_BLOCK_KEY", "ef01")
	t.Setenv("FLOW_ALLOWED_SUBS", "user-a, user-b,user-c")

	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Config{
		Addr:             ":9000",
		BaseURL:          "https://flow.example.com",
		OIDCIssuer:       "https://auth.example.com/realms/flow",
		OIDCClientID:     "flow-server",
		OIDCClientSecret: "secret",
		CookieHashKey:    "abcd",
		CookieBlockKey:   "ef01",
		AllowedSubs:      []string{"user-a", "user-b", "user-c"},
	}
	if got.Addr != want.Addr || got.BaseURL != want.BaseURL ||
		got.OIDCIssuer != want.OIDCIssuer || got.OIDCClientID != want.OIDCClientID ||
		got.OIDCClientSecret != want.OIDCClientSecret ||
		got.CookieHashKey != want.CookieHashKey || got.CookieBlockKey != want.CookieBlockKey ||
		!slices.Equal(got.AllowedSubs, want.AllowedSubs) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}
