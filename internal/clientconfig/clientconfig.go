// Package clientconfig loads flow CLI/TUI configuration from the environment.
package clientconfig

import "fmt"

type Config struct {
	ServerURL   string
	OIDCIssuer  string
	CliClientID string
}

// Load reads config via getenv (injected for testability). ServerURL and
// CliClientID have dev-friendly defaults; OIDCIssuer is required.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		ServerURL:   getenv("FLOW_SERVER_URL"),
		OIDCIssuer:  getenv("FLOW_OIDC_ISSUER"),
		CliClientID: getenv("FLOW_OIDC_CLI_CLIENT_ID"),
	}
	if c.ServerURL == "" {
		c.ServerURL = "http://localhost:8080"
	}
	if c.CliClientID == "" {
		c.CliClientID = "flow-cli"
	}
	if c.OIDCIssuer == "" {
		return Config{}, fmt.Errorf("clientconfig: FLOW_OIDC_ISSUER is required")
	}
	return c, nil
}
