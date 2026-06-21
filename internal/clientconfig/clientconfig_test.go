package clientconfig

import "testing"

func TestLoadDefaults(t *testing.T) {
	env := map[string]string{"FLOW_OIDC_ISSUER": "http://localhost:5556/dex"}
	c := Load(func(k string) string { return env[k] })
	// Dev default is https (flow-server serves self-signed TLS for browser h2).
	if c.ServerURL != "https://localhost:8080" {
		t.Fatalf("ServerURL default: %q", c.ServerURL)
	}
	// A self-signed cert on loopback is dev → trust it without FLOW_INSECURE_TLS.
	if !c.InsecureTLS {
		t.Fatalf("InsecureTLS should be auto-true for https://localhost")
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
	// A real (non-loopback) https server must NOT auto-skip verification.
	if c.InsecureTLS {
		t.Fatal("InsecureTLS must stay false for a non-loopback https server")
	}
}

func TestLoadInsecureTLS(t *testing.T) {
	// loopback variants auto-enable; explicit env still works; plain http never.
	cases := []struct {
		serverURL, insecureEnv string
		want                   bool
	}{
		{"https://localhost:8080", "", true},
		{"https://127.0.0.1:8080", "", true},
		{"https://[::1]:8080", "", true},
		{"http://localhost:8080", "", false},        // loopback but not TLS
		{"https://flow.example.com", "", false},     // TLS but not loopback
		{"https://flow.example.com", "1", true},     // explicit override
	}
	for _, tc := range cases {
		env := map[string]string{"FLOW_SERVER_URL": tc.serverURL, "FLOW_INSECURE_TLS": tc.insecureEnv}
		c := Load(func(k string) string { return env[k] })
		if c.InsecureTLS != tc.want {
			t.Errorf("InsecureTLS for %q (env=%q) = %v, want %v", tc.serverURL, tc.insecureEnv, c.InsecureTLS, tc.want)
		}
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
	if c.ServerURL != "https://localhost:8080" || c.CliClientID != "flow-cli" {
		t.Fatalf("defaults not applied with empty env: %+v", c)
	}
}
