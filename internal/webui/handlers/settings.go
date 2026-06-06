// Package handlers — see dashboard.go for the per-handler-Deps
// convention. The settings handler is mounted at /settings and shows
// the logged-in user's identity, server endpoint info, and build
// metadata. Logout is wired to POST /logout (handled by the M1 auth
// middleware).
package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	settingstmpl "github.com/serverkraken/flow/internal/webui/templates/settings"
)

// SettingsDeps bundles exactly the data sources the /settings handler
// needs. Most data comes off the request context (cookie sub/email/
// name via SessionValueFromContext) or static config (server URL, OIDC
// issuer, server DB path). Phase 2 will grow this struct when device
// telemetry + sync state become available.
//
// StartTime is the moment flow-server booted — used for the Uptime
// readout in the Version section. Plumb the real boot timestamp from
// cmd/flow-server/main.go (Task 10 wires this).
type SettingsDeps struct {
	ServerBaseURL string
	OIDCIssuer    string
	ServerDBPath  string
	StartTime     time.Time
	Clock         ports.Clock
}

// buildInfoOnce caches runtime/debug.ReadBuildInfo so we don't re-read
// it on every request. The result is the trimmed commit SHA (first 7
// chars) read from vcs.revision and the Go module version. Both are
// best-effort — when build info is unavailable (e.g. running via `go
// test`), the WebUI falls back to "dev".
var (
	buildInfoOnce sync.Once
	cachedCommit  string
	cachedGoVer   string
)

// resolveBuildInfo returns (commitShortSHA, goVersion) lazily. Empty
// strings when ReadBuildInfo returns nothing (test binaries) — the VM
// builder substitutes "dev" so the field never blanks.
func resolveBuildInfo() (string, string) {
	buildInfoOnce.Do(func() {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		cachedGoVer = info.GoVersion
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				rev := strings.TrimSpace(s.Value)
				if len(rev) > 7 {
					rev = rev[:7]
				}
				cachedCommit = rev
				break
			}
		}
	})
	return cachedCommit, cachedGoVer
}

// NewSettings returns the http.Handler mounted at /settings. The
// BrowserAuthMiddleware guarantees a domain.User + a session-value
// payload (sub/email/name) in context; the handler fails closed with
// 401 if either is absent.
func NewSettings(d SettingsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Defensive 404 for any subpath — Phase 2 may add /settings/{section}.
		tail := strings.TrimPrefix(r.URL.Path, "/settings")
		tail = strings.TrimPrefix(tail, "/")
		if tail != "" {
			http.NotFound(w, r)
			return
		}

		sub, email, name, _ := httpserver.SessionValueFromContext(r.Context())
		// Prefer the cookie's payload; fall back to the resolved User
		// record when the session payload lacks a field. This also keeps
		// external-package tests usable without poking the unexported
		// sessionValue struct — they wire WithUser only and the fallback
		// fills the strip.
		if sub == "" {
			sub = u.OIDCSub
		}
		if email == "" {
			email = u.Email
		}
		if name == "" {
			name = u.DisplayName
		}

		now := d.Clock.Now()
		commit, goVer := resolveBuildInfo()

		vm := settingstmpl.IndexVM{
			Eyebrow:       email,
			Identity:      buildIdentitySection(sub, email, name, d.OIDCIssuer),
			DeviceLabel:   deviceLabelFrom(r),
			ServerBaseURL: orFallback(d.ServerBaseURL, "(nicht konfiguriert)"),
			ServerDBPath:  orFallback(d.ServerDBPath, "(nicht konfiguriert)"),
			Phase:         "Phase 1 · M6/M7",
			CommitShort:   orFallback(commit, "dev"),
			GoVersion:     orFallback(goVer, "—"),
			Uptime:        formatUptime(now.Sub(d.StartTime)),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		meta := layout.PageMeta{
			Title:       "Settings",
			CurrentPath: "/settings",
			UserLabel:   userLabelFromContext(r.Context()),
			Spine:       layout.SpineState{SyncState: "ok"},
		}
		if err := layout.Base(meta, settingstmpl.Index(vm)).Render(r.Context(), w); err != nil {
			slog.Error(
				"settings: render failed",
				slog.String("user_id", u.ID),
				slog.String("error", err.Error()),
			)
		}
	})
}

// buildIdentitySection populates the Identität section's key-value
// rows. Empty cookie fields render as "—" so the strip never looks
// half-broken. Scopes and cookie expiry are placeholders for Phase 2.
func buildIdentitySection(sub, email, name, issuer string) settingstmpl.IdentitySection {
	return settingstmpl.IdentitySection{
		Email:       orFallback(email, "—"),
		Sub:         orFallback(sub, "—"),
		DisplayName: orFallback(name, "—"),
		Issuer:      orFallback(issuer, "(nicht konfiguriert)"),
		// Cookie expiry: the secure-cookie codec doesn't expose its
		// expiry to the handler. Phase 2 will widen middleware to plumb
		// it through; for now show a placeholder so the row stays
		// honest about the gap.
		CookieExpiryLabel: "Phase 2",
		Scopes:            "openid · email",
	}
}

// deviceLabelFrom builds the placeholder Geräte row label. We don't
// have device telemetry yet, so we use the request's User-Agent as a
// shorthand. Phase 2: real device-id + Lamport counter.
func deviceLabelFrom(r *http.Request) string {
	ua := strings.TrimSpace(r.Header.Get("User-Agent"))
	if ua == "" {
		return "(aktuelles gerät)"
	}
	if len(ua) > 64 {
		ua = ua[:61] + "…"
	}
	return ua
}

// formatUptime renders a duration as "Nd Nh Nm" / "Nh Nm" / "Nm" so
// the Version section's Uptime row stays honest about short-lived
// instances (CI, tests, fresh boots).
func formatUptime(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	mins := int(d / time.Minute)
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

// orFallback returns s if non-empty, else fallback. Centralised so
// every section uses the same "—" / "(nicht konfiguriert)" convention.
func orFallback(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
