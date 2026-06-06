package usecase_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// mustOpenServerStore opens an in-memory-style server Store under a temp dir
// and registers a cleanup. Mirrors the sqliteserver package's mustOpenServer
// helper (kept package-private there), so each test owns an isolated DB.
func mustOpenServerStore(t *testing.T) *sqliteserver.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := sqliteserver.Open(dir + "/server.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// seedUser inserts a user via the public Users adapter and returns it.
func seedUser(t *testing.T, store *sqliteserver.Store, suffix string) domain.User {
	t.Helper()
	u, err := sqliteserver.NewUsers(store).EnsureBySub("sub|"+suffix, suffix+"@example.com", suffix)
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}
	return u
}

// seedProject inserts a project for userID via the public Projects adapter.
func seedProject(t *testing.T, store *sqliteserver.Store, userID, slug string) domain.Project {
	t.Helper()
	p, err := sqliteserver.NewProjects(store).EnsureBySlug(userID, slug, slug)
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}
	return p
}

// seedSession inserts a session for (user, project) at date+offset with dur.
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
	}
	out, err := sessions.Upsert(in, 0)
	if err != nil {
		t.Fatalf("Upsert session: %v", err)
	}
	return out
}

// mkView wires a ServerWorktimeView pinned to "now" with an 8h default target.
func mkView(t *testing.T, store *sqliteserver.Store, now time.Time) *usecase.ServerWorktimeView {
	t.Helper()
	return &usecase.ServerWorktimeView{
		Sessions:      sqliteserver.NewSessions(store),
		Active:        sqliteserver.NewActiveSessions(store),
		Clock:         &testutil.FixedClock{T: now},
		DefaultTarget: 8 * time.Hour,
	}
}

// — Today —

func TestServerWorktimeView_Today_EmptyDay_Weekday_8hTarget(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "swv-today1")
	// 2026-04-29 was a Wednesday.
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	v := mkView(t, store, now)

	day, err := v.Today(u.ID)
	if err != nil {
		t.Fatalf("Today: %v", err)
	}
	if len(day.Sessions) != 0 {
		t.Errorf("want 0 sessions, got %d", len(day.Sessions))
	}
	if day.Logged != 0 {
		t.Errorf("want 0 logged, got %v", day.Logged)
	}
	if day.Active != nil {
		t.Errorf("want no active, got %v", day.Active)
	}
	if day.PausedAt != nil {
		t.Errorf("server has no pause state; want nil, got %v", day.PausedAt)
	}
	if day.Target != 8*time.Hour {
		t.Errorf("Target on weekday: got %v, want 8h", day.Target)
	}
}

func TestServerWorktimeView_Today_Weekend_ZeroTarget(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "swv-weekend")
	// 2026-05-02 was a Saturday.
	now := time.Date(2026, 5, 2, 11, 0, 0, 0, time.UTC)
	v := mkView(t, store, now)

	day, err := v.Today(u.ID)
	if err != nil {
		t.Fatalf("Today: %v", err)
	}
	if day.Target != 0 {
		t.Errorf("weekend target: got %v, want 0", day.Target)
	}
}

func TestServerWorktimeView_Today_TwoSessionsPlusActive(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "swv-today2")
	p := seedProject(t, store, u.ID, "swv-proj")
	sessions := sqliteserver.NewSessions(store)

	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC), 2*time.Hour)
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 4, 29, 11, 30, 0, 0, time.UTC), 90*time.Minute)
	// Yesterday's session: must be excluded from Today.
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC), 8*time.Hour)

	// Start an active session at 13:30 today.
	if _, err := sqliteserver.NewActiveSessions(store).Start(u.ID, p.ID, "laptop", 0, "", ""); err != nil {
		t.Fatalf("Start active: %v", err)
	}

	v := mkView(t, store, now)
	day, err := v.Today(u.ID)
	if err != nil {
		t.Fatalf("Today: %v", err)
	}
	if len(day.Sessions) != 2 {
		t.Errorf("want 2 today sessions, got %d", len(day.Sessions))
	}
	if day.Logged != 3*time.Hour+30*time.Minute {
		t.Errorf("Logged: got %v, want 3h30m", day.Logged)
	}
	if day.Active == nil {
		t.Error("want Active != nil after Start")
	}
}

// — Week —

func TestServerWorktimeView_Week_SpansMonToSun_FlagsToday(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "swv-week1")
	p := seedProject(t, store, u.ID, "swv-week-proj")
	sessions := sqliteserver.NewSessions(store)

	// Wednesday 2026-04-29: week is Mon 27 - Sun 03/May.
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	// Monday 04-27 + Wednesday 04-29 sessions.
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 4, 27, 9, 0, 0, 0, time.UTC), 4*time.Hour)
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC), 2*time.Hour)

	v := mkView(t, store, now)
	week, err := v.Week(u.ID)
	if err != nil {
		t.Fatalf("Week: %v", err)
	}
	// Weekend dropped (no sessions + not today) → 5 weekdays.
	if len(week) != 5 {
		t.Errorf("want 5 weekday rows, got %d", len(week))
	}

	var foundToday bool
	var todayLogged time.Duration
	for _, wd := range week {
		if wd.IsToday {
			foundToday = true
			todayLogged = wd.Logged
			if !domain.SameDay(wd.Date, now) {
				t.Errorf("IsToday row date %v, want %v", wd.Date, now)
			}
		}
	}
	if !foundToday {
		t.Error("no row flagged IsToday")
	}
	if todayLogged != 2*time.Hour {
		t.Errorf("today logged: got %v, want 2h", todayLogged)
	}
}

func TestServerWorktimeView_Week_ShowWeekendKeepsSatSun(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "swv-week2")
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)

	v := mkView(t, store, now)
	v.ShowWeekend = true

	week, err := v.Week(u.ID)
	if err != nil {
		t.Fatalf("Week: %v", err)
	}
	if len(week) != 7 {
		t.Errorf("want 7 rows with ShowWeekend, got %d", len(week))
	}
}

// — History —

func TestServerWorktimeView_History_LastSixtyDays_SortedDesc(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "swv-hist1")
	p := seedProject(t, store, u.ID, "swv-hist-proj")
	sessions := sqliteserver.NewSessions(store)

	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	// Three days inside the 60-day window.
	for i, off := range []int{-1, -5, -30} {
		start := time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC).AddDate(0, 0, off)
		seedSession(t, sessions, u.ID, p.ID, start, time.Duration(i+1)*time.Hour)
	}
	// One day outside the window (≥ 61 days back).
	seedSession(t, sessions, u.ID, p.ID, time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC).AddDate(0, 0, -90), time.Hour)

	v := mkView(t, store, now)
	hist, err := v.History(u.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("want 3 history rows (60-day window), got %d", len(hist))
	}
	// Sorted desc by Date.
	for i := 1; i < len(hist); i++ {
		if !hist[i-1].Date.After(hist[i].Date) {
			t.Errorf("history not desc-sorted at %d: %v before %v", i, hist[i-1].Date, hist[i].Date)
		}
	}
}

// — Range —

func TestServerWorktimeView_Range_FiltersByDateWindow(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "swv-range1")
	p := seedProject(t, store, u.ID, "swv-range-proj")
	sessions := sqliteserver.NewSessions(store)

	base := time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC)
	for i := -2; i <= 2; i++ {
		seedSession(t, sessions, u.ID, p.ID, base.AddDate(0, 0, i), time.Hour)
	}

	v := mkView(t, store, base)
	got, err := v.Range(u.ID, base.AddDate(0, 0, -1), base.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("Range: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("want 3 sessions in 3-day window, got %d", len(got))
	}
}

// — userID isolation —

func TestServerWorktimeView_UserIsolation(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	uA := seedUser(t, store, "swv-isoA")
	uB := seedUser(t, store, "swv-isoB")
	pA := seedProject(t, store, uA.ID, "swv-iso-pA")
	pB := seedProject(t, store, uB.ID, "swv-iso-pB")
	sessions := sqliteserver.NewSessions(store)

	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	seedSession(t, sessions, uA.ID, pA.ID, time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC), 3*time.Hour)
	seedSession(t, sessions, uB.ID, pB.ID, time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC), 5*time.Hour)
	// uB starts an active session that must NOT show on uA's Today.
	if _, err := sqliteserver.NewActiveSessions(store).Start(uB.ID, pB.ID, "phone", 0, "", ""); err != nil {
		t.Fatalf("Start active uB: %v", err)
	}

	v := mkView(t, store, now)
	dayA, err := v.Today(uA.ID)
	if err != nil {
		t.Fatalf("Today uA: %v", err)
	}
	if dayA.Logged != 3*time.Hour {
		t.Errorf("uA Logged leaked uB data: got %v, want 3h", dayA.Logged)
	}
	if dayA.Active != nil {
		t.Errorf("uA Active leaked uB's running session")
	}
	for _, s := range dayA.Sessions {
		if s.UserID != uA.ID {
			t.Errorf("uA.Today returned row with userID %q", s.UserID)
		}
	}

	histA, err := v.History(uA.ID)
	if err != nil {
		t.Fatalf("History uA: %v", err)
	}
	for _, rec := range histA {
		for _, s := range rec.Sessions {
			if s.UserID != uA.ID {
				t.Errorf("uA.History returned row with userID %q", s.UserID)
			}
		}
	}
}
