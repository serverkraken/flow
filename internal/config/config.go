// Package config loads flow-server configuration from the environment.
package config

import (
	"fmt"
	"strings"
)

type Config struct {
	DatabaseURL  string
	OIDCIssuer   string
	OIDCClientID string
	AllowedSubs  map[string]bool
	ListenAddr   string
	Dev          bool
}

// Load reads config via getenv (injected for testability).
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		DatabaseURL:  getenv("DATABASE_URL"),
		OIDCIssuer:   getenv("FLOW_OIDC_ISSUER"),
		OIDCClientID: getenv("FLOW_OIDC_CLIENT_ID"),
		ListenAddr:   getenv("FLOW_LISTEN_ADDR"),
		Dev:          getenv("FLOW_DEV") == "1",
		AllowedSubs:  map[string]bool{},
	}
	for _, s := range strings.Split(getenv("FLOW_ALLOWED_SUBS"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			c.AllowedSubs[s] = true
		}
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	for k, v := range map[string]string{"DATABASE_URL": c.DatabaseURL, "FLOW_OIDC_ISSUER": c.OIDCIssuer, "FLOW_OIDC_CLIENT_ID": c.OIDCClientID} {
		if v == "" {
			return Config{}, fmt.Errorf("config: %s is required", k)
		}
	}
	return c, nil
}
