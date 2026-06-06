package httpserver

import (
	"os"
	"strings"
)

// Config holds all flow-server configuration read from environment variables.
// One struct, one source of truth — flags would require a parser and we don't
// need them yet. All env-vars use the FLOW_ prefix.
type Config struct {
	Addr             string   // FLOW_SERVER_ADDR (default :8080)
	BaseURL          string   // FLOW_SERVER_BASE_URL (default http://localhost:8080)
	OIDCIssuer       string   // FLOW_OIDC_ISSUER (Authentik realm URL)
	OIDCClientID     string   // FLOW_OIDC_CLIENT_ID
	OIDCClientSecret string   // FLOW_OIDC_CLIENT_SECRET
	CookieHashKey    string   // FLOW_COOKIE_HASH_KEY (hex, 64 chars = 32 bytes)
	CookieBlockKey   string   // FLOW_COOKIE_BLOCK_KEY (hex, 32 or 64 chars)
	AllowedSubs      []string // FLOW_ALLOWED_SUBS (comma-separated OIDC 'sub' values)
	ServerDBPath     string   // FLOW_SERVER_DB (default /var/lib/flow/server.db)
	// NotebookRoot points at a kompendium notebook directory the WebUI
	// renders read-only under /notes. Empty (default) — the /notes
	// handler shows a "Notes nicht konfiguriert" placeholder instead of
	// 500ing. Phase 1 is single-user (allowed-subs gate), so one notebook
	// root per server is sufficient. Wired in cmd/flow-server/main.go
	// (Plan E · Task 10).
	NotebookRoot string // FLOW_NOTEBOOK_ROOT (empty default → /notes shows placeholder)
}

// LoadConfig reads the configuration from environment variables. Returns an
// error reserved for future validation (Phase-1 has none yet; keep the
// signature so callers don't need to change when validation arrives).
func LoadConfig() (Config, error) {
	return Config{
		Addr:             envOrDefault("FLOW_SERVER_ADDR", ":8080"),
		BaseURL:          envOrDefault("FLOW_SERVER_BASE_URL", "http://localhost:8080"),
		OIDCIssuer:       os.Getenv("FLOW_OIDC_ISSUER"),
		OIDCClientID:     os.Getenv("FLOW_OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("FLOW_OIDC_CLIENT_SECRET"),
		CookieHashKey:    os.Getenv("FLOW_COOKIE_HASH_KEY"),
		CookieBlockKey:   os.Getenv("FLOW_COOKIE_BLOCK_KEY"),
		AllowedSubs:      splitCSV(os.Getenv("FLOW_ALLOWED_SUBS")),
		ServerDBPath:     envOrDefault("FLOW_SERVER_DB", "/var/lib/flow/server.db"),
		NotebookRoot:     os.Getenv("FLOW_NOTEBOOK_ROOT"),
	}, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
