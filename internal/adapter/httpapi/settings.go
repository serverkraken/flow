package httpapi

// Settings fetches/persists user settings from the bearer API.
//
// NewConfigReader composes an ini-based ConfigReader with server settings so
// the server's "daily_target" key overrides the local file's value.

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Settings adapter wraps the /api/v1/settings endpoint.
type Settings struct {
	c *Client
}

// NewSettings constructs a Settings adapter backed by c.
func NewSettings(c *Client) *Settings { return &Settings{c: c} }

// Get returns all user settings as a key/value map.
func (s *Settings) Get(ctx context.Context) (map[string]string, error) {
	var out map[string]string
	if err := s.c.doJSON(ctx, http.MethodGet, "/api/v1/settings", nil, -1, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = make(map[string]string)
	}
	return out, nil
}

// Put replaces the user settings map on the server.
func (s *Settings) Put(ctx context.Context, m map[string]string) error {
	return s.c.doJSON(ctx, http.MethodPut, "/api/v1/settings", m, -1, nil)
}

// — composedConfigReader ------------------------------------------------------

// composedConfigReader merges a local ini-based ConfigReader with server
// settings. The server's "daily_target" overrides the local config.
type composedConfigReader struct {
	settings *Settings
	ini      ports.ConfigReader
}

// NewConfigReader returns a ports.ConfigReader that merges ini with server
// settings. If the server has no "daily_target" yet it is seeded from the
// ini-file's DefaultTarget (idempotent first-run behaviour).
func NewConfigReader(c *Client, ini ports.ConfigReader) ports.ConfigReader {
	return &composedConfigReader{settings: NewSettings(c), ini: ini}
}

// Load returns the merged configuration.
func (r *composedConfigReader) Load() (domain.Config, error) {
	cfg, err := r.ini.Load()
	if err != nil {
		return domain.Config{}, err
	}
	ctx := context.Background()
	serverMap, err := r.settings.Get(ctx)
	if err != nil {
		// Server unreachable — fall back to ini config silently
		slog.Warn("httpapi: settings fetch failed — using local config", "err", err)
		return cfg, nil
	}
	if len(serverMap) == 0 {
		// First run: seed server with ini default target
		if cfg.DefaultTarget > 0 {
			seed := map[string]string{
				"daily_target": cfg.DefaultTarget.String(),
			}
			if err := r.settings.Put(ctx, seed); err != nil {
				slog.Warn("httpapi: failed to seed daily_target on server", "err", err)
			} else {
				slog.Info("httpapi: seeded daily_target on server",
					"value", cfg.DefaultTarget.String())
			}
		}
		return cfg, nil
	}
	if raw, ok := serverMap["daily_target"]; ok {
		d, err := time.ParseDuration(raw)
		if err == nil {
			cfg.DefaultTarget = d
		} else {
			slog.Warn("httpapi: bad daily_target from server — keeping local value",
				"raw", raw, "err", err)
		}
	}
	return cfg, nil
}
