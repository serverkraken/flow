// Package config loads flow-server configuration from the environment.
package config

import (
	"fmt"
	"strings"
)

type Config struct {
	DatabaseURL      string
	OIDCIssuer       string
	OIDCClientID     string
	OIDCCliClientID  string
	OIDCClientSecret string
	PublicBaseURL    string
	SessionSecret    string
	AllowedSubs      map[string]bool
	ListenAddr       string
	Dev              bool
}

// Load reads config via getenv (injected for testability).
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		DatabaseURL:      getenv("DATABASE_URL"),
		OIDCIssuer:       getenv("FLOW_OIDC_ISSUER"),
		OIDCClientID:     getenv("FLOW_OIDC_CLIENT_ID"),
		OIDCCliClientID:  getenv("FLOW_OIDC_CLI_CLIENT_ID"),
		OIDCClientSecret: getenv("FLOW_OIDC_CLIENT_SECRET"),
		PublicBaseURL:    getenv("FLOW_PUBLIC_BASE_URL"),
		SessionSecret:    getenv("FLOW_SESSION_SECRET"),
		ListenAddr:       getenv("FLOW_LISTEN_ADDR"),
		Dev:              getenv("FLOW_DEV") == "1",
		AllowedSubs:      map[string]bool{},
	}
	for _, s := range strings.Split(getenv("FLOW_ALLOWED_SUBS"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			c.AllowedSubs[s] = true
		}
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	for _, f := range []struct{ name, val string }{
		{"DATABASE_URL", c.DatabaseURL},
		{"FLOW_OIDC_ISSUER", c.OIDCIssuer},
		{"FLOW_OIDC_CLIENT_ID", c.OIDCClientID},
		{"FLOW_OIDC_CLI_CLIENT_ID", c.OIDCCliClientID},
		{"FLOW_OIDC_CLIENT_SECRET", c.OIDCClientSecret},
		{"FLOW_PUBLIC_BASE_URL", c.PublicBaseURL},
		{"FLOW_SESSION_SECRET", c.SessionSecret},
	} {
		if f.val == "" {
			return Config{}, fmt.Errorf("config: %s is required", f.name)
		}
	}
	return c, nil
}

// RedirectURL is the OIDC auth-code callback, derived from the public base URL.
func (c Config) RedirectURL() string {
	return strings.TrimRight(c.PublicBaseURL, "/") + "/auth/callback"
}
