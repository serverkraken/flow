package httpserver

import (
	"os"
	"strings"
	"time"
)

// Config holds all flow-server configuration read from environment variables.
// One struct, one source of truth — flags would require a parser and we don't
// need them yet. All env-vars use the FLOW_ prefix.
type Config struct {
	Addr       string // FLOW_SERVER_ADDR (default :8080)
	BaseURL    string // FLOW_SERVER_BASE_URL (default http://localhost:8080)
	OIDCIssuer string // FLOW_OIDC_ISSUER (Authentik realm URL)
	// OIDCCLIIssuer is the issuer URL of the public CLI/MCP device-flow
	// provider. Authentik runs per_provider issuer mode, so the flow-cli
	// Application mints tokens with iss=.../o/flow-cli/ — distinct from the
	// browser .../o/flow/ — and signs them against its own JWKS. flow-server
	// stands up a second verifier for it (see oidcserver.Provider).
	//
	// Defaults to FLOW_OIDC_ISSUER when unset: single-issuer IdPs (the local
	// dex stack) need only one value, and an Authentik deploy that hasn't set
	// this yet keeps booting (CLI tokens stay rejected exactly as before — no
	// boot crash, no regression) until the env is added. This is a same-URL
	// fallback, NOT slug auto-derivation, which would silently paper over a
	// misconfigured issuer.
	OIDCCLIIssuer    string // FLOW_OIDC_CLI_ISSUER (default: FLOW_OIDC_ISSUER)
	OIDCClientID     string // FLOW_OIDC_CLIENT_ID (browser auth-code, confidential)
	OIDCClientSecret string // FLOW_OIDC_CLIENT_SECRET (browser auth-code)
	// OIDCCLIClientID is the public OIDC client used by CLI/MCP device-flow.
	// Separate from OIDCClientID because the CLI cannot ship a client_secret
	// — Authentik must register this as a `public` client with grant type
	// `urn:ietf:params:oauth:grant-type:device_code`. Default `flow-cli`
	// matches the legacy hardcoded constant and keeps existing deployments
	// working without an env change.
	OIDCCLIClientID string   // FLOW_OIDC_CLI_CLIENT_ID (default "flow-cli")
	CookieHashKey   string   // FLOW_COOKIE_HASH_KEY (hex, 64 chars = 32 bytes)
	CookieBlockKey  string   // FLOW_COOKIE_BLOCK_KEY (hex, 32 or 64 chars)
	AllowedSubs     []string // FLOW_ALLOWED_SUBS (comma-separated OIDC 'sub' values)
	// PgDSN is the PostgreSQL connection string — the server's ONLY
	// truth store after the R1 rebuild (Spec §4). Required.
	// Beispiel: postgres://flow:secret@flow-pg-rw:5432/flow?sslmode=disable
	PgDSN string // FLOW_PG_DSN (Pflicht)
	// ShutdownTimeout caps how long flow-server drains in-flight HTTP
	// requests after SIGTERM before forcing exit. Tuned at 30s by default
	// — long enough that a slow OIDC-callback or templ render can finish,
	// short enough that K8s' default 30s grace period (configurable via
	// terminationGracePeriodSeconds) doesn't escalate to SIGKILL.
	// FLOW_SERVER_SHUTDOWN_TIMEOUT (Go duration string, e.g. "30s", "1m").
	ShutdownTimeout time.Duration // FLOW_SERVER_SHUTDOWN_TIMEOUT (default 30s)
}

// LoadConfig reads the configuration from environment variables. Returns an
// error reserved for future validation (Phase-1 has none yet; keep the
// signature so callers don't need to change when validation arrives).
func LoadConfig() (Config, error) {
	return Config{
		Addr:             envOrDefault("FLOW_SERVER_ADDR", ":8080"),
		BaseURL:          envOrDefault("FLOW_SERVER_BASE_URL", "http://localhost:8080"),
		OIDCIssuer:       os.Getenv("FLOW_OIDC_ISSUER"),
		OIDCCLIIssuer:    envOrDefault("FLOW_OIDC_CLI_ISSUER", os.Getenv("FLOW_OIDC_ISSUER")),
		OIDCClientID:     os.Getenv("FLOW_OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("FLOW_OIDC_CLIENT_SECRET"),
		OIDCCLIClientID:  envOrDefault("FLOW_OIDC_CLI_CLIENT_ID", "flow-cli"),
		CookieHashKey:    os.Getenv("FLOW_COOKIE_HASH_KEY"),
		CookieBlockKey:   os.Getenv("FLOW_COOKIE_BLOCK_KEY"),
		AllowedSubs:      splitCSV(os.Getenv("FLOW_ALLOWED_SUBS")),
		PgDSN:            os.Getenv("FLOW_PG_DSN"),
		ShutdownTimeout:  envDurationOrDefault("FLOW_SERVER_SHUTDOWN_TIMEOUT", 30*time.Second),
	}, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envDurationOrDefault parses a Go duration string (e.g. "30s", "2m") from
// the given env var. Empty or malformed values fall back to def — we
// prefer "boot with safe default" over "crash on a typo" for an
// operational knob like shutdown timeout.
func envDurationOrDefault(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
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
