// flow-server is the multi-device sync HTTP server for flow. See
// docs/superpowers/specs/2026-06-02-flow-client-server-phase1-design.md for
// the M1 design.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/oidcserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
)

// cliClientID is the OIDC client used by the CLI/MCP device-flow. Separate
// from the server's confidential client (FLOW_OIDC_CLIENT_ID) because the
// CLI is a public client without a client secret.
//
// Phase-1 keeps this hardcoded; Phase 2 will make it configurable via
// FLOW_OIDC_CLI_CLIENT_ID once we support multiple CLI installs.
const cliClientID = "flow-cli"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := httpserver.LoadConfig()
	if err != nil {
		logger.Error("config load failed", slog.Any("err", err))
		os.Exit(1)
	}
	if err := requireConfig(cfg); err != nil {
		logger.Error("config validation failed", slog.Any("err", err))
		os.Exit(1)
	}

	// --- SQLite server store -------------------------------------------------

	if err := os.MkdirAll(filepath.Dir(cfg.ServerDBPath), 0o755); err != nil {
		logger.Error("server db dir", slog.Any("err", err))
		os.Exit(1)
	}
	serverDB, err := sqliteserver.Open(cfg.ServerDBPath)
	if err != nil {
		logger.Error("open server db", slog.Any("err", err))
		os.Exit(1)
	}
	defer func() {
		if err := serverDB.Close(); err != nil {
			logger.Error("server db close", slog.Any("err", err))
		}
	}()

	users := sqliteserver.NewUsers(serverDB)
	projects := sqliteserver.NewProjects(serverDB)
	sessions := sqliteserver.NewSessions(serverDB)
	activeStore := sqliteserver.NewActiveSessions(serverDB)

	// --- OIDC + session cookie -----------------------------------------------

	ctx := context.Background()
	provider, err := oidcserver.NewProvider(ctx, oidcserver.ProviderConfig{
		Issuer:   cfg.OIDCIssuer,
		ClientID: cfg.OIDCClientID,
	})
	if err != nil {
		logger.Error("oidc provider init failed", slog.Any("err", err))
		os.Exit(1)
	}

	access := oidcserver.NewSubAllowlist(cfg.AllowedSubs)

	session, err := httpserver.NewSessionFromHex(cfg.CookieHashKey, cfg.CookieBlockKey)
	if err != nil {
		logger.Error("session keys invalid", slog.Any("err", err))
		os.Exit(1)
	}

	_, tokenURL := provider.Endpoint()
	oidcCfg := httpserver.OIDCConfigResponse{
		Issuer:                 cfg.OIDCIssuer,
		DeviceAuthorizationURL: provider.DeviceAuthorizationURL(),
		TokenURL:               tokenURL,
		ClientID:               cliClientID,
	}

	secure := strings.HasPrefix(cfg.BaseURL, "https://")
	srv := httpserver.NewWithAuth(httpserver.AuthDeps{
		Provider:       provider,
		Access:         access,
		Session:        session,
		Users:          users,
		ProjectsServer: projects,
		SessionsServer: sessions,
		ActiveServer:   activeStore,
		BaseURL:        cfg.BaseURL,
		OIDCClientID:   cfg.OIDCClientID,
		OIDCSecret:     cfg.OIDCClientSecret,
		Cookie:         httpserver.CookieConfig{Name: "flow_session", Secure: secure},
		Ready:          func() error { return serverDB.DB().Ping() },
		OIDCConfig:     oidcCfg,
	})

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Info("flow-server starting",
		slog.String("addr", cfg.Addr),
		slog.String("base_url", cfg.BaseURL),
		slog.String("issuer", cfg.OIDCIssuer),
		slog.Int("allowed_subs", len(cfg.AllowedSubs)),
	)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server crashed", slog.Any("err", err))
		os.Exit(1)
	}
}

// requireConfig fails fast if the operator forgot a required env var. Better
// to crash at boot than to serve 500s on every /login.
func requireConfig(c httpserver.Config) error {
	var missing []string
	if c.OIDCIssuer == "" {
		missing = append(missing, "FLOW_OIDC_ISSUER")
	}
	if c.OIDCClientID == "" {
		missing = append(missing, "FLOW_OIDC_CLIENT_ID")
	}
	if c.OIDCClientSecret == "" {
		missing = append(missing, "FLOW_OIDC_CLIENT_SECRET")
	}
	if c.CookieHashKey == "" {
		missing = append(missing, "FLOW_COOKIE_HASH_KEY")
	}
	if c.CookieBlockKey == "" {
		missing = append(missing, "FLOW_COOKIE_BLOCK_KEY")
	}
	if len(c.AllowedSubs) == 0 {
		missing = append(missing, "FLOW_ALLOWED_SUBS")
	}
	if len(missing) > 0 {
		return errors.New("missing required env vars: " + strings.Join(missing, ", "))
	}
	return nil
}
