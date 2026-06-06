package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/serverkraken/flow/internal/webui/handlers"
)

func mkWorktimeDeps(store *sqliteserver.Store, now time.Time) handlers.WorktimeDeps {
	clock := &testutil.FixedClock{T: now}
	view := &usecase.ServerWorktimeView{
		Sessions:      sqliteserver.NewSessions(store),
		Active:        sqliteserver.NewActiveSessions(store),
		Clock:         clock,
		DefaultTarget: 8 * time.Hour,
	}
	return handlers.WorktimeDeps{
		View:        view,
		Active:      sqliteserver.NewActiveSessions(store),
		Sessions:    sqliteserver.NewSessions(store),
		Projects:    sqliteserver.NewProjects(store),
		Clock:       clock,
		DeviceLabel: "mac-soenne",
	}
}

func wtReq(t *testing.T, target string, withUser bool, store *sqliteserver.Store, suffix string) (*http.Request, *sqliteserver.Store) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	if withUser {
		u := seedUser(t, store, suffix)
		ctx := httpserver.WithUser(r.Context(), u)
		r = r.WithContext(ctx)
	}
	return r, store
}

// — Heute —

func TestWorktime_TabHeute_WithActiveSession_RendersBannerAndTable(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "wt-heute-active")
	p := seedProject(t, store, u.ID, "webui-mockups")
	sessions := sqliteserver.NewSessions(store)

	// Thursday 14:00.
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 90*time.Minute)
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 4, 11, 0, 0, 0, time.UTC), 30*time.Minute)
	active := sqliteserver.NewActiveSessions(store)
	if _, err := active.Start(u.ID, p.ID, "mac-soenne", 0, "design", "Editorial-Terminal Mockups"); err != nil {
		t.Fatalf("Start active: %v", err)
	}

	h := handlers.NewWorktime(mkWorktimeDeps(store, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=heute", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), u))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain := []string{
		"webui-mockups",          // project name appears
		"aktive session",         // live banner eyebrow
		"sessions-table",         // table rendered
		"data-testid=\"daybar\"", // rail daybar
		"saldo-stripe",           // saldo-stripe class
		"Sessions heute",         // section heading
		"Heute · Ist",            // first saldo cell label
		"Saldo · Jahr",           // third saldo cell label
		`class="subtab is-active"`, // active sub-tab
	}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("heute body missing %q", s)
		}
	}
}

func TestWorktime_TabHeute_EmptyDay_RendersPlaceholder(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "wt-heute-empty")
	// Friday 11:00 — work day, no sessions.
	now := time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC)

	h := handlers.NewWorktime(mkWorktimeDeps(store, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=heute", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), u))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Noch keine Sessions heute") {
		t.Errorf("empty-state placeholder missing")
	}
	if strings.Contains(body, "aktive session") {
		t.Errorf("live-banner eyebrow leaked into empty render")
	}
}

// — Woche —

func TestWorktime_TabWoche_RendersChartAndSaldo(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "wt-woche")
	p := seedProject(t, store, u.ID, "flow")
	sessions := sqliteserver.NewSessions(store)

	// Pin to Thursday so Mon-Wed of the same ISO week have sessions.
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), 7*time.Hour+30*time.Minute)
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC), 8*time.Hour)
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), 6*time.Hour+45*time.Minute)

	h := handlers.NewWorktime(mkWorktimeDeps(store, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=woche", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), u))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain := []string{
		`id="week-chart-data"`,                // JSON block exists
		`type="application/json"`,             // JSON block type
		`initWeekChart`,                       // init script reference
		`/static/charts-init.js`,              // js include
		"Verteilung · Mo — So",                // chart section heading
		"Nach Projekt",                         // proj list heading
		"Nach Tag",                            // tag list heading
		"12-Wochen-Saldo",                     // rail head
		`id="week-saldo-data"`,                // sparkline JSON block
		"Ist · Woche",                         // saldo stripe label
		"Soll · Woche",                        // saldo stripe label
	}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("woche body missing %q", s)
		}
	}
	// 7 bars in the week chart JSON → seven `"label"` keys.
	labelCount := strings.Count(body, `"label":`)
	if labelCount < 7 {
		t.Errorf("week-chart-data: got %d label entries, want ≥ 7 (one per weekday)", labelCount)
	}
}

// — Verlauf —

func TestWorktime_TabVerlauf_WithDate_RendersSelectedDay(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "wt-verlauf-date")
	p := seedProject(t, store, u.ID, "kompendium")
	sessions := sqliteserver.NewSessions(store)

	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC), 3*time.Hour)

	h := handlers.NewWorktime(mkWorktimeDeps(store, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=verlauf&date=2026-06-02", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), u))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "kompendium") {
		t.Errorf("verlauf: project name missing from selected-day render")
	}
	if !strings.Contains(body, `data-testid="history-table"`) {
		t.Errorf("verlauf: sessions table missing")
	}
	if !strings.Contains(body, "tab=verlauf&amp;date=2026-06-01") {
		t.Errorf("verlauf: prev-day link missing")
	}
}

func TestWorktime_TabVerlauf_MissingDate_DefaultsToYesterday(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "wt-verlauf-default")
	p := seedProject(t, store, u.ID, "flow")
	sessions := sqliteserver.NewSessions(store)

	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	// Seed yesterday with a single session — verifies default = yesterday.
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC), 90*time.Minute)

	h := handlers.NewWorktime(mkWorktimeDeps(store, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=verlauf", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), u))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	body := rr.Body.String()
	// Yesterday = 2026-06-03 = Mittwoch.
	if !strings.Contains(body, "Mittwoch · 03. Juni 2026") {
		t.Errorf("verlauf default-day: missing yesterday label\n%s", body[:min(800, len(body))])
	}
	if !strings.Contains(body, "flow") {
		t.Errorf("verlauf default-day: project name missing")
	}
}

func TestWorktime_TabVerlauf_InvalidDate_FallsBackToYesterday(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "wt-verlauf-invalid")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := handlers.NewWorktime(mkWorktimeDeps(store, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=verlauf&date=not-a-date", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), u))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("invalid date should NOT 400; got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Mittwoch · 03. Juni 2026") {
		t.Errorf("invalid date should fall back to yesterday")
	}
}

// — Frei —

func TestWorktime_TabFrei_RendersPhase2Placeholder(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "wt-frei")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := handlers.NewWorktime(mkWorktimeDeps(store, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=frei", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), u))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Frei-Tage werden in Phase 2 server-seitig synchronisiert") {
		t.Errorf("frei placeholder text missing")
	}
	if !strings.Contains(body, `data-testid="frei-placeholder"`) {
		t.Errorf("frei placeholder testid missing")
	}
}

// — Default + invalid tab fall through to heute —

func TestWorktime_DefaultTab_IsHeute(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "wt-default")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := handlers.NewWorktime(mkWorktimeDeps(store, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), u))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Sessions heute") {
		t.Errorf("default tab should render Heute (Sessions heute heading missing)")
	}
}

func TestWorktime_InvalidTab_FallsThroughToHeute(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "wt-garbage")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := handlers.NewWorktime(mkWorktimeDeps(store, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=garbage", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), u))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("garbage tab should not 400; got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Sessions heute") {
		t.Errorf("garbage tab should fall through to Heute")
	}
}

// — Auth —

func TestWorktime_MissingUser_Returns401(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := handlers.NewWorktime(mkWorktimeDeps(store, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=heute", nil).WithContext(context.Background())
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing user: got %d, want 401", rr.Code)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
