// Package clientconfig loads flow CLI/TUI configuration from the environment.
package clientconfig

import "net/url"

type Config struct {
	ServerURL   string
	OIDCIssuer  string
	CliClientID string
	// InsecureTLS skips server-certificate verification. It is set by
	// FLOW_INSECURE_TLS=1, and auto-enabled for an https loopback server (the
	// dev stack's self-signed flow-server on localhost) so a bare `flow ui`
	// works without extra env. Never true for a non-loopback https server
	// unless FLOW_INSECURE_TLS=1 is set explicitly.
	InsecureTLS bool
}

// Load reads config via getenv (injected for testability). ServerURL and
// CliClientID have dev-friendly defaults. OIDCIssuer is optional here: it is
// only required for `flow login` and for refreshing an expired token, so it is
// enforced at those points of use rather than eagerly (a valid stored token
// works without it).
func Load(getenv func(string) string) Config {
	c := Config{
		ServerURL:   getenv("FLOW_SERVER_URL"),
		OIDCIssuer:  getenv("FLOW_OIDC_ISSUER"),
		CliClientID: getenv("FLOW_OIDC_CLI_CLIENT_ID"),
		InsecureTLS: getenv("FLOW_INSECURE_TLS") == "1",
	}
	if c.ServerURL == "" {
		// Dev default is https: flow-server serves self-signed TLS in dev so the
		// browser negotiates HTTP/2 (real deployments set FLOW_SERVER_URL).
		c.ServerURL = "https://localhost:8080"
	}
	if c.CliClientID == "" {
		c.CliClientID = "flow-cli"
	}
	// A self-signed cert on a loopback host is definitionally local dev; trust it
	// without requiring FLOW_INSECURE_TLS so a bare `flow ui` works in dev.
	if isLoopbackHTTPS(c.ServerURL) {
		c.InsecureTLS = true
	}
	return c
}

// isLoopbackHTTPS reports whether raw is an https URL pointing at a loopback
// host (localhost / 127.0.0.1 / ::1).
func isLoopbackHTTPS(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
