package clientconfig

import "testing"

func TestLoadDefaults(t *testing.T) {
	env := map[string]string{"FLOW_OIDC_ISSUER": "http://localhost:5556/dex"}
	c, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
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
	c, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if c.ServerURL != "https://flow.example.com" || c.CliClientID != "custom-cli" {
		t.Fatalf("overrides not applied: %+v", c)
	}
}

func TestLoadMissingIssuer(t *testing.T) {
	if _, err := Load(func(string) string { return "" }); err == nil {
		t.Fatal("expected error when FLOW_OIDC_ISSUER unset")
	}
}
