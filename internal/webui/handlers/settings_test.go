package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/webui/handlers"
)

// mkSettingsDeps assembles SettingsDeps with reasonable defaults that
// the tests can override per-case.
func mkSettingsDeps(now, start time.Time) handlers.SettingsDeps {
	return handlers.SettingsDeps{
		ServerBaseURL: "https://flow.example.test",
		OIDCIssuer:    "https://authentik.example.test/application/o/flow/",
		ServerDBPath:  "/var/lib/flow/server.db",
		StartTime:     start,
		Clock:         &testutil.FixedClock{T: now},
	}
}

// settingsReqWithUser builds a /settings request with a resolved
// domain.User attached to context. The handler falls back to User
// fields when SessionValueFromContext lacks them, so external-package
// tests can wire identity via WithUser alone without needing to poke
// the unexported cookie session-value struct.
func settingsReqWithUser(t *testing.T, u domain.User) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/settings", nil)
	r.Header.Set("User-Agent", "test-agent/1.0")
	return r.WithContext(httpserver.WithUser(r.Context(), u))
}

func TestSettings_RendersIdentityFromContext(t *testing.T) {
	t.Parallel()
	u := domain.User{ID: "uid-1", OIDCSub: "sub|alice", Email: "alice@example.test", DisplayName: "Alice"}
	now := time.Date(2026, 6, 6, 18, 0, 0, 0, time.UTC)
	start := now.Add(-(3*24*time.Hour + 14*time.Hour + 22*time.Minute))

	h := handlers.NewSettings(mkSettingsDeps(now, start))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, settingsReqWithUser(t, u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain := []string{
		`data-testid="settings-identity"`,
		"alice@example.test",     // email shows in Identität + eyebrow
		"sub|alice",              // sub shows in Identität
		"Alice",                  // display name
		"authentik.example.test", // issuer URL
		"openid · email",         // hardcoded scopes
		"Phase 1 · M6/M7",        // Phase label
		"3d 14h 22m",             // uptime label
	}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("settings body missing %q", s)
		}
	}
}

func TestSettings_ServerURLAndDBPath(t *testing.T) {
	t.Parallel()
	u := domain.User{ID: "uid-2", OIDCSub: "sub|bob", Email: "bob@example.test"}
	now := time.Date(2026, 6, 6, 18, 0, 0, 0, time.UTC)
	d := mkSettingsDeps(now, now.Add(-time.Hour))
	d.ServerBaseURL = "https://my-flow.serverkraken.io"
	d.ServerDBPath = "/srv/flow/state.db"

	h := handlers.NewSettings(d)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, settingsReqWithUser(t, u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "https://my-flow.serverkraken.io") {
		t.Errorf("server base URL missing from sync section")
	}
	if !strings.Contains(body, "/srv/flow/state.db") {
		t.Errorf("server.db path missing from export section")
	}
	// Sanity: the four other section markers should be present.
	for _, marker := range []string{
		`data-testid="settings-devices"`,
		`data-testid="settings-sync"`,
		`data-testid="settings-export"`,
		`data-testid="settings-version"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("section marker missing: %s", marker)
		}
	}
}

func TestSettings_LogoutFormPostsToLogout(t *testing.T) {
	t.Parallel()
	u := domain.User{ID: "uid-3", OIDCSub: "sub|carol", Email: "carol@example.test"}
	now := time.Date(2026, 6, 6, 18, 0, 0, 0, time.UTC)

	h := handlers.NewSettings(mkSettingsDeps(now, now.Add(-time.Minute)))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, settingsReqWithUser(t, u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	// The logout button MUST be a real form POST — not disabled.
	wantSnippet := `<form method="post" action="/logout"`
	if !strings.Contains(body, wantSnippet) {
		t.Errorf("logout form missing — expected snippet %q in body", wantSnippet)
	}
	// And it must not be disabled.
	idx := strings.Index(body, `data-testid="settings-logout-form"`)
	if idx < 0 {
		t.Fatalf("logout form testid not found")
	}
	tail := body[idx:]
	closeIdx := strings.Index(tail, "</form>")
	if closeIdx < 0 {
		t.Fatalf("logout form close tag not found")
	}
	formChunk := tail[:closeIdx]
	if strings.Contains(formChunk, `disabled`) || strings.Contains(formChunk, `aria-disabled="true"`) {
		t.Errorf("logout button is disabled but must be active; form=%q", formChunk)
	}
	if !strings.Contains(formChunk, "Abmelden") {
		t.Errorf("logout button label missing; form=%q", formChunk)
	}
}

func TestSettings_VersionSection_HasBuildInfo(t *testing.T) {
	t.Parallel()
	u := domain.User{ID: "uid-4", OIDCSub: "sub|dave"}
	now := time.Date(2026, 6, 6, 18, 0, 0, 0, time.UTC)

	h := handlers.NewSettings(mkSettingsDeps(now, now.Add(-time.Hour-10*time.Minute)))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, settingsReqWithUser(t, u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `data-testid="settings-version"`) {
		t.Errorf("version section missing")
	}
	// Uptime line must show the formatted boot delta.
	if !strings.Contains(body, "1h 10m") {
		t.Errorf("uptime label '1h 10m' missing from version section")
	}
	if !strings.Contains(body, "flow-server commit") {
		t.Errorf("commit row label missing")
	}
}

func TestSettings_MissingUser_Returns401(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 6, 18, 0, 0, 0, time.UTC)
	h := handlers.NewSettings(mkSettingsDeps(now, now.Add(-time.Minute)))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/settings", nil).WithContext(context.Background())
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing user on /settings: got %d, want 401", rr.Code)
	}
}
