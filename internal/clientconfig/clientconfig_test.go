package clientconfig

import "testing"

func TestLoadDefaults(t *testing.T) {
	env := map[string]string{"FLOW_OIDC_ISSUER": "http://localhost:5556/dex"}
	c := Load(func(k string) string { return env[k] })
	if c.ServerURL != "http://localhost:8080" {
		t.Fatalf("ServerURL default: %q", c.ServerURL)
	}
	if c.CliClientID != "flow-cli" {
		t.Fatalf("CliClientID default: %q", c.CliClientID)
	}
	if c.OIDCIssuer != "http://localhost:5556/dex" {
		t.Fatalf("OIDCIssuer: %q", c.OIDCIssuer)
	}
}

func TestLoadOverrides(t *testing.T) {
	env := map[string]string{
		"FLOW_SERVER_URL":         "https://flow.example.com",
		"FLOW_OIDC_ISSUER":        "https://id.example.com/o/flow/",
		"FLOW_OIDC_CLI_CLIENT_ID": "custom-cli",
	}
	c := Load(func(k string) string { return env[k] })
	if c.ServerURL != "https://flow.example.com" || c.CliClientID != "custom-cli" {
		t.Fatalf("overrides not applied: %+v", c)
	}
}

// The issuer is optional: a valid stored access token can be used without it
// (the issuer is only needed for login and for the refresh path). Load still
// applies the ServerURL/CliClientID defaults and never errors.
func TestLoadIssuerOptional(t *testing.T) {
	c := Load(func(string) string { return "" })
	if c.OIDCIssuer != "" {
		t.Fatalf("expected empty issuer, got %q", c.OIDCIssuer)
	}
	if c.ServerURL != "http://localhost:8080" || c.CliClientID != "flow-cli" {
		t.Fatalf("defaults not applied with empty env: %+v", c)
	}
}
