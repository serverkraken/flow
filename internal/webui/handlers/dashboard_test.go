package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/serverkraken/flow/internal/webui/handlers"
	"github.com/serverkraken/flow/internal/webui/templates/dashboard"
)

// — helpers (kept private to this file; mirror the patterns from
//   internal/usecase/server_worktime_view_test.go) —

func mustOpenServerStore(t *testing.T) *sqliteserver.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := sqliteserver.Open(dir + "/srv.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func seedUser(t *testing.T, store *sqliteserver.Store, suffix string) domain.User {
	t.Helper()
	u, err := sqliteserver.NewUsers(store).EnsureBySub("sub|"+suffix, suffix+"@example.com", suffix)
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}
	return u
}

func seedProject(t *testing.T, store *sqliteserver.Store, userID, name string) domain.Project {
	t.Helper()
	p, err := sqliteserver.NewProjects(store).EnsureBySlug(userID, name, name)
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}
	return p
}

func seedSession(t *testing.T, sessions *sqliteserver.Sessions, userID, projectID string, start time.Time, dur time.Duration) domain.Session {
	t.Helper()
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	in := domain.Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		ProjectID: projectID,
		Date:      day,
		Start:     start,
		Stop:      start.Add(dur),
		Elapsed:   dur,
		Tag:       "design",
	}
	out, err := sessions.Upsert(in, 0)
	if err != nil {
		t.Fatalf("Upsert session: %v", err)
	}
	return out
}

func mkDeps(store *sqliteserver.Store, now time.Time) handlers.DashboardDeps {
	clock := &testutil.FixedClock{T: now}
	view := &usecase.ServerWorktimeView{
		Sessions:      sqliteserver.NewSessions(store),
		Active:        sqliteserver.NewActiveSessions(store),
		Clock:         clock,
		DefaultTarget: 8 * time.Hour,
	}
	return handlers.DashboardDeps{
		View:     view,
		Active:   sqliteserver.NewActiveSessions(store),
		Sessions: sqliteserver.NewSessions(store),
		Projects: sqliteserver.NewProjects(store),
		Clock:    clock,
	}
}

func reqWithUser(t *testing.T, u domain.User) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := httpserver.WithUser(r.Context(), u)
	return r.WithContext(ctx)
}

// — handler tests —

func TestDashboard_RendersExpectedShape(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "dash1")
	p := seedProject(t, store, u.ID, "webui-mockups")
	sessions := sqliteserver.NewSessions(store)

	// 2026-06-04 = Thursday — work day with 8h target.
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	// Two completed sessions earlier today.
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), 90*time.Minute)
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 6, 4, 11, 0, 0, 0, time.UTC), 30*time.Minute)
	// One active session running since 13:00 (~1h ago at "now").
	active := sqliteserver.NewActiveSessions(store)
	if _, err := active.Start(u.ID, p.ID, time.Time{}, "laptop", 0, "design", ""); err != nil {
		t.Fatalf("Start active: %v", err)
	}

	h := handlers.NewDashboard(mkDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reqWithUser(t, u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	mustContain := []string{
		"webui-mockups", // project name in live banner / activity row
		"▶",             // active marker glyph
		"daybar",        // day-bar section rendered
		"data-testid=\"daybar\"",
		"Heute · Übersicht",   // tiny header title
		"aktive session",      // banner eyebrow
		"Aktivität",           // section heading
		"Top Projekt · Woche", // mini-card eyebrow
		"Sessions Woche",      // mini-card eyebrow
	}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("body missing %q", s)
		}
	}

	// 24 day-bar cells. We render one `<div class="cell"...` per hour.
	cellCount := strings.Count(body, `<div class="cell`)
	if cellCount != 24 {
		t.Errorf("day-bar cells: got %d, want 24", cellCount)
	}
}

func TestDashboard_NoActiveSession_NoLiveBanner(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "dash-quiet")
	// Sunday — no target. No sessions.
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	h := handlers.NewDashboard(mkDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reqWithUser(t, u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "aktive session") {
		t.Errorf("banner eyebrow leaked into quiet-day render")
	}
	if !strings.Contains(body, "Noch keine Sessions diese Woche.") {
		t.Errorf("empty activity placeholder missing")
	}
}

// TestDashboard_SSEEventNamesAreRegistered guards the wiring that turns the
// SSE EventSource into htmx:sseMessage events. The htmx-sse extension only
// registers per-event-name listeners when an element carries sse-swap (or
// hx-trigger="sse:…"); without such a marker the named events arrive on the
// EventSource but never reach the inline script, so the dashboard never
// reloads after session.stopped — the active session looked like it kept
// running in the UI even after the user pressed Stop.
func TestDashboard_SSEEventNamesAreRegistered(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "dash-sse")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := handlers.NewDashboard(mkDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reqWithUser(t, u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	// The whole point: sse-swap (or hx-trigger="sse:…") MUST appear on
	// some descendant of the sse-connect wrapper, otherwise no listeners
	// are bound. If you change the wiring, make sure each event name the
	// inline sseDashboardScript() switches on is covered here.
	for _, name := range []string{
		"session.started", "session.stopped", "session.updated", "session.deleted",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("SSE event %q is not registered on the dashboard — htmx-sse will swallow it silently", name)
		}
	}
	if !strings.Contains(body, "sse-swap=") && !strings.Contains(body, `hx-trigger="sse:`) {
		t.Errorf("dashboard SSE wrapper has no sse-swap or hx-trigger=\"sse:…\" marker; htmx:sseMessage will never fire")
	}
}

func TestDashboard_MissingUser_Returns401(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	h := handlers.NewDashboard(mkDeps(store, now))
	rr := httptest.NewRecorder()
	// No user injected — the defensive branch must fire.
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(context.Background())
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing user: got %d, want 401", rr.Code)
	}
}

// — helper-level tests (don't touch the templ surface) —
//
// HourMask / FormatHHMM / FormatSignedHHMM / HumanRelativeTime tests
// live in internal/webui/format/ alongside the helpers themselves.

func TestTopProjectOfWeek(t *testing.T) {
	t.Parallel()
	sessions := []domain.Session{
		{ProjectID: "p-flow", Elapsed: 3 * time.Hour},
		{ProjectID: "p-stalwart", Elapsed: 90 * time.Minute},
		{ProjectID: "p-flow", Elapsed: 2 * time.Hour},
	}
	id, total := dashboard.TopProjectOfWeek(sessions)
	if id != "p-flow" {
		t.Errorf("id: got %q, want p-flow", id)
	}
	if total != 5*time.Hour {
		t.Errorf("total: got %v, want 5h", total)
	}
}

func TestBuildActivityStream_SortsAndLimits(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	sessions := []domain.Session{
		{
			ProjectID: "p1", Stop: now.Add(-3 * time.Hour),
			Elapsed: time.Hour, Tag: "review",
		},
		{
			ProjectID: "p2", Stop: now.Add(-5 * time.Hour),
			Elapsed: 30 * time.Minute,
		},
	}
	active := &domain.ActiveSession{
		ProjectID: "p1",
		StartedAt: now.Add(-1 * time.Hour),
		Tag:       "design",
	}
	resolver := func(id string) string { return "Proj-" + id }
	events := dashboard.BuildActivityStream(sessions, active, resolver, 7, now)

	if len(events) != 3 {
		t.Fatalf("event count: got %d, want 3", len(events))
	}
	// Active is the most recent → must be first.
	if events[0].Kind != dashboard.ActivitySessionStarted {
		t.Errorf("first event kind: got %v, want SessionStarted", events[0].Kind)
	}
	// Order must be newest-first.
	for i := 1; i < len(events); i++ {
		if events[i].When.After(events[i-1].When) {
			t.Errorf("events not newest-first at i=%d", i)
		}
	}
}
