package config

import "testing"

func TestLoadFromEnv(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":            "postgres://flow:flow@localhost:5432/flow?sslmode=disable",
		"FLOW_OIDC_ISSUER":        "https://id.thebackend.org/application/o/flow/",
		"FLOW_OIDC_CLIENT_ID":     "flow",
		"FLOW_OIDC_CLI_CLIENT_ID": "flow-cli",
		"FLOW_ALLOWED_SUBS":       "msoent, alice",
		"FLOW_LISTEN_ADDR":        ":8080",
		"FLOW_DEV":                "1",
		"FLOW_OIDC_CLIENT_SECRET": "shh",
		"FLOW_PUBLIC_BASE_URL":    "https://flow.thebackend.org",
		"FLOW_SESSION_SECRET":     "0123456789abcdef0123456789abcdef",
	}
	c, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DatabaseURL == "" || c.OIDCIssuer == "" {
		t.Fatal("required fields empty")
	}
	if len(c.AllowedSubs) != 2 || !c.AllowedSubs["alice"] {
		t.Fatalf("allowlist parse: %v", c.AllowedSubs)
	}
	if !c.Dev {
		t.Fatal("dev flag not parsed")
	}
	if c.OIDCClientSecret != "shh" || c.SessionSecret == "" {
		t.Fatal("auth-code config not parsed")
	}
	if c.OIDCCliClientID != "flow-cli" {
		t.Fatalf("CLI client id not parsed: %q", c.OIDCCliClientID)
	}
	if got := c.RedirectURL(); got != "https://flow.thebackend.org/auth/callback" {
		t.Fatalf("RedirectURL = %q", got)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	if _, err := Load(func(string) string { return "" }); err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
}

func TestLoadDefaultsListenAddr(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":            "postgres://x",
		"FLOW_OIDC_ISSUER":        "https://issuer",
		"FLOW_OIDC_CLIENT_ID":     "flow",
		"FLOW_OIDC_CLI_CLIENT_ID": "flow-cli",
		"FLOW_OIDC_CLIENT_SECRET": "s",
		"FLOW_PUBLIC_BASE_URL":    "https://flow.example.com",
		"FLOW_SESSION_SECRET":     "secret",
	}
	c, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ListenAddr != ":8080" {
		t.Fatalf("ListenAddr default: got %q, want :8080", c.ListenAddr)
	}
}
