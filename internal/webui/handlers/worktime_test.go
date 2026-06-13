package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func mkWorktimeDeps(s pgStores, now time.Time) WorktimeDeps {
	clock := &testutil.FixedClock{T: now}
	view := &usecase.ServerWorktimeView{
		Sessions:      s.Sessions,
		Active:        s.Active,
		Clock:         clock,
		DefaultTarget: 8 * time.Hour,
	}
	return WorktimeDeps{
		View:        view,
		Active:      s.Active,
		Sessions:    s.Sessions,
		Projects:    s.Projects,
		Clock:       clock,
		DeviceLabel: "mac-soenne",
	}
}

func seedProjectForWorktime(t *testing.T, projects *pgstore.Projects, userID, name string) string {
	t.Helper()
	p, err := projects.EnsureBySlug(userID, name, name)
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}
	return p.ID
}

// — Heute —

func TestWorktime_TabHeute_WithActiveSession_RendersBannerAndTable(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "wt-heute-active")
	pID := seedProjectForWorktime(t, s.Projects, s.User.ID, "webui-mockups")

	// Thursday 14:00.
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	seedSession(t, s.Sessions, s.User.ID, pID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 90*time.Minute)
	seedSession(t, s.Sessions, s.User.ID, pID, time.Date(2026, 6, 4, 11, 0, 0, 0, time.UTC), 30*time.Minute)
	if _, err := s.Active.Start(s.User.ID, pID, time.Time{}, "mac-soenne", 0, "design", "Editorial-Terminal Mockups"); err != nil {
		t.Fatalf("Start active: %v", err)
	}

	h := NewWorktime(mkWorktimeDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=heute", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), s.User))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"webui-mockups",            // project name appears
		"aktive session",           // live banner eyebrow
		"sessions-table",           // table rendered
		"data-testid=\"daybar\"",   // rail daybar
		"saldo-stripe",             // saldo-stripe class
		"Sessions heute",           // section heading
		"Heute · Ist",              // first saldo cell label
		"Saldo · Jahr",             // third saldo cell label
		`class="subtab is-active"`, // active sub-tab
	} {
		if !strings.Contains(body, want) {
			t.Errorf("heute body missing %q", want)
		}
	}
}

func TestWorktime_TabHeute_EmptyDay_RendersPlaceholder(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "wt-heute-empty")
	// Friday 11:00 — work day, no sessions.
	now := time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC)

	h := NewWorktime(mkWorktimeDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=heute", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), s.User))
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
	s := newPGStores(t, "wt-woche")
	pID := seedProjectForWorktime(t, s.Projects, s.User.ID, "flow")

	// Pin to Thursday so Mon-Wed of the same ISO week have sessions.
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	seedSession(t, s.Sessions, s.User.ID, pID, time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), 7*time.Hour+30*time.Minute)
	seedSession(t, s.Sessions, s.User.ID, pID, time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC), 8*time.Hour)
	seedSession(t, s.Sessions, s.User.ID, pID, time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), 6*time.Hour+45*time.Minute)

	h := NewWorktime(mkWorktimeDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=woche", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), s.User))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		`id="week-chart-data"`,    // JSON block exists
		`type="application/json"`, // JSON block type
		`initWeekChart`,           // init script reference
		`/static/charts-init.js`,  // js include
		"Verteilung · Mo — So",    // chart section heading
		"Nach Projekt",            // proj list heading
		"Nach Tag",                // tag list heading
		"12-Wochen-Saldo",         // rail head
		`id="week-saldo-data"`,    // sparkline JSON block
		"Ist · Woche",             // saldo stripe label
		"Soll · Woche",            // saldo stripe label
	} {
		if !strings.Contains(body, want) {
			t.Errorf("woche body missing %q", want)
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
	s := newPGStores(t, "wt-verlauf-date")
	pID := seedProjectForWorktime(t, s.Projects, s.User.ID, "kompendium")

	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	seedSession(t, s.Sessions, s.User.ID, pID, time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC), 3*time.Hour)

	h := NewWorktime(mkWorktimeDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=verlauf&date=2026-06-02", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), s.User))
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
	s := newPGStores(t, "wt-verlauf-default")
	pID := seedProjectForWorktime(t, s.Projects, s.User.ID, "flow-vd")

	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	// Seed yesterday with a single session — verifies default = yesterday.
	seedSession(t, s.Sessions, s.User.ID, pID, time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC), 90*time.Minute)

	h := NewWorktime(mkWorktimeDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=verlauf", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), s.User))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	body := rr.Body.String()
	// Yesterday = 2026-06-03 = Mittwoch.
	if !strings.Contains(body, "Mittwoch · 03. Juni 2026") {
		t.Errorf("verlauf default-day: missing yesterday label\n%s", body[:min(800, len(body))])
	}
	if !strings.Contains(body, "flow-vd") {
		t.Errorf("verlauf default-day: project name missing")
	}
	// Jump-header uses relative-day vocabulary: for date=yesterday the
	// strip should read "< vorgestern · gestern · heute >".
	if !strings.Contains(body, "vorgestern") {
		t.Errorf("verlauf default-day: PrevLabel should be 'vorgestern'\nbody=%s", body[:min(1200, len(body))])
	}
	if !strings.Contains(body, "gestern") {
		t.Errorf("verlauf default-day: SelectedLabel should be 'gestern'")
	}
	if !strings.Contains(body, "heute") {
		t.Errorf("verlauf default-day: NextLabel should be 'heute' (since selected=yesterday, next=today)")
	}
}

func TestWorktime_TabVerlauf_FarPastDate_UsesShortGermanFormat(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "wt-verlauf-far-past")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := NewWorktime(mkWorktimeDeps(s, now))
	rr := httptest.NewRecorder()
	// 2026-01-15 is Thursday → "Do · 15.01.2026".
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=verlauf&date=2026-01-15", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), s.User))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	body := rr.Body.String()
	// SelectedLabel should fall back to short German date for a date
	// > 2 days away from now.
	if !strings.Contains(body, "Do · 15.01.2026") {
		t.Errorf("verlauf far-past: SelectedLabel should be 'Do · 15.01.2026'\nbody=%s", body[:min(1200, len(body))])
	}
}

func TestWorktime_TabVerlauf_InvalidDate_FallsBackToYesterday(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "wt-verlauf-invalid")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := NewWorktime(mkWorktimeDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=verlauf&date=not-a-date", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), s.User))
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
	s := newPGStores(t, "wt-frei")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := NewWorktime(mkWorktimeDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=frei", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), s.User))
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
	s := newPGStores(t, "wt-default")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := NewWorktime(mkWorktimeDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), s.User))
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
	s := newPGStores(t, "wt-garbage")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := NewWorktime(mkWorktimeDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=garbage", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), s.User))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("garbage tab should not 400; got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Sessions heute") {
		t.Errorf("garbage tab should fall through to Heute")
	}
}

// — SSE wiring —

// TestWorktime_TabHeute_SSEEventNamesAreRegistered guards the wiring that
// turns the SSE EventSource into htmx:sseMessage events on worktime/today.
func TestWorktime_TabHeute_SSEEventNamesAreRegistered(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "wt-heute-sse")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := NewWorktime(mkWorktimeDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=heute", nil)
	r = r.WithContext(httpserver.WithUser(r.Context(), s.User))
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, name := range []string{
		"tick",
		"session.started", "session.stopped", "session.updated", "session.deleted",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("SSE event %q is not registered on /worktime?tab=heute — htmx-sse will swallow it silently", name)
		}
	}
	if !strings.Contains(body, "sse-swap=") && !strings.Contains(body, `hx-trigger="sse:`) {
		t.Errorf("today SSE wrapper has no sse-swap or hx-trigger=\"sse:…\" marker; htmx:sseMessage will never fire")
	}
}

// — Auth —

func TestWorktime_MissingUser_Returns401(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "wt-nouser")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := NewWorktime(mkWorktimeDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/worktime?tab=heute", nil).WithContext(context.Background())
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing user: got %d, want 401", rr.Code)
	}
}
