// Package clientconfig loads flow CLI/TUI configuration from the environment.
package clientconfig

type Config struct {
	ServerURL   string
	OIDCIssuer  string
	CliClientID string
	// InsecureTLS skips server-certificate verification. Set FLOW_INSECURE_TLS=1
	// ONLY for the dev stack, whose flow-server presents a self-signed cert so
	// the browser can use HTTP/2. Never enable against a real deployment.
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
		c.ServerURL = "http://localhost:8080"
	}
	if c.CliClientID == "" {
		c.CliClientID = "flow-cli"
	}
	return c
}
