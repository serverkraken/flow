package config

import "testing"

func TestLoadFromEnv(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":        "postgres://flow:flow@localhost:5432/flow?sslmode=disable",
		"FLOW_OIDC_ISSUER":    "https://id.thebackend.org/application/o/flow/",
		"FLOW_OIDC_CLIENT_ID": "flow",
		"FLOW_ALLOWED_SUBS":   "msoent, alice",
		"FLOW_LISTEN_ADDR":    ":8080",
		"FLOW_DEV":            "1",
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
}

func TestLoadMissingRequired(t *testing.T) {
	if _, err := Load(func(string) string { return "" }); err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
}

func TestLoadDefaultsListenAddr(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":        "postgres://x",
		"FLOW_OIDC_ISSUER":    "https://issuer",
		"FLOW_OIDC_CLIENT_ID": "flow",
	}
	c, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ListenAddr != ":8080" {
		t.Fatalf("ListenAddr default: got %q, want :8080", c.ListenAddr)
	}
}
