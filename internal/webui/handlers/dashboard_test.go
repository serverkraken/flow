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
	if _, err := active.Start(u.ID, p.ID, "laptop", 0, "design", ""); err != nil {
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
		"webui-mockups",       // project name in live banner / activity row
		"▶",                   // active marker glyph
		"daybar",              // day-bar section rendered
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

func TestHourMask_MarksHoursTouched(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 4, 14, 30, 0, 0, time.UTC)
	sess := []domain.Session{
		{
			Start: time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC),
			Stop:  time.Date(2026, 6, 4, 10, 30, 0, 0, time.UTC),
		},
		{
			Start: time.Date(2026, 6, 4, 13, 0, 0, 0, time.UTC),
			Stop:  time.Date(2026, 6, 4, 13, 45, 0, 0, time.UTC),
		},
	}
	mask := dashboard.HourMask(sess, nil, now)

	worked := []int{9, 10, 13}
	for _, h := range worked {
		if mask[h] != 1 {
			t.Errorf("hour %d: got %d, want 1", h, mask[h])
		}
	}
	if mask[11] != 0 || mask[12] != 0 {
		t.Errorf("non-worked hours got marked: 11=%d 12=%d", mask[11], mask[12])
	}
	// Hour 14 has no completed session, no active → must remain 0.
	if mask[14] != 0 {
		t.Errorf("hour 14 incorrectly marked")
	}
}

func TestHourMask_ActiveExtendsMaskToNow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 4, 14, 30, 0, 0, time.UTC)
	active := time.Date(2026, 6, 4, 13, 0, 0, 0, time.UTC)
	mask := dashboard.HourMask(nil, &active, now)

	for _, h := range []int{13, 14} {
		if mask[h] != 1 {
			t.Errorf("active hour %d: got %d, want 1", h, mask[h])
		}
	}
	if mask[12] != 0 || mask[15] != 0 {
		t.Errorf("non-active hours got marked: 12=%d 15=%d", mask[12], mask[15])
	}
}

func TestFormatHHMM(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0:00"},
		{30 * time.Minute, "0:30"},
		{8 * time.Hour, "8:00"},
		{8*time.Hour + 14*time.Minute, "8:14"},
		{-time.Hour, "0:00"}, // clamped
	}
	for _, c := range cases {
		if got := dashboard.FormatHHMM(c.in); got != c.want {
			t.Errorf("FormatHHMM(%v): got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatSignedHHMM(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0:00"},
		{30 * time.Minute, "+0:30"},
		{-30 * time.Minute, "-0:30"},
		{12*time.Hour + 42*time.Minute, "+12:42"},
		{-(8*time.Hour + 0*time.Minute), "-8:00"},
	}
	for _, c := range cases {
		if got := dashboard.FormatSignedHHMM(c.in); got != c.want {
			t.Errorf("FormatSignedHHMM(%v): got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHumanRelativeTime(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"8s ago", now.Add(-8 * time.Second), "vor 8s"},
		{"2m ago", now.Add(-2 * time.Minute), "vor 2m"},
		{"today 09:28", time.Date(2026, 6, 4, 9, 28, 0, 0, time.UTC), "heute · 09:28"},
		{"yesterday", time.Date(2026, 6, 3, 17, 45, 0, 0, time.UTC), "gestern · 17:45"},
		{"2 days ago", time.Date(2026, 6, 2, 14, 12, 0, 0, time.UTC), "vor 2 Tagen · 14:12"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dashboard.HumanRelativeTime(c.in, now); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

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
