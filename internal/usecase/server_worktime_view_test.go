package usecase_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// — in-memory fakes -----------------------------------------------------------

// fakeSessionsReader holds a flat slice; ListByUserDateRange filters by user +
// date window. Satisfies usecase.ServerSessionsReader.
type fakeSessionsReader struct {
	sessions []domain.Session
}

func (f fakeSessionsReader) ListByUserDateRange(userID string, from, to time.Time) ([]domain.Session, error) {
	var out []domain.Session
	for _, s := range f.sessions {
		if s.UserID != userID {
			continue
		}
		day := time.Date(s.Date.Year(), s.Date.Month(), s.Date.Day(), 0, 0, 0, 0, time.UTC)
		fromDay := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
		toDay := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
		if !day.Before(fromDay) && !day.After(toDay) {
			out = append(out, s)
		}
	}
	return out, nil
}

// fakeActiveReader holds active sessions per user.
// Satisfies usecase.ServerActiveSessionsReader.
type fakeActiveReader struct {
	rows []domain.ActiveSession
}

func (f fakeActiveReader) ListByUser(userID string) ([]domain.ActiveSession, error) {
	var out []domain.ActiveSession
	for _, a := range f.rows {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, nil
}

// — helpers -------------------------------------------------------------------

// makeSession constructs a domain.Session for a given (userID, projectID,
// start, elapsed). Assigns a random ID and sets Date to the UTC day of start.
func makeSession(userID, projectID string, start time.Time, elapsed time.Duration) domain.Session {
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	return domain.Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		ProjectID: projectID,
		Date:      day,
		Start:     start,
		Stop:      start.Add(elapsed),
		Elapsed:   elapsed,
	}
}

// mkView wires a ServerWorktimeView pinned to "now" with an 8h default target.
func mkView(sessions fakeSessionsReader, active fakeActiveReader, now time.Time) *usecase.ServerWorktimeView {
	return &usecase.ServerWorktimeView{
		Sessions:      sessions,
		Active:        active,
		Clock:         &testutil.FixedClock{T: now},
		DefaultTarget: 8 * time.Hour,
	}
}

// — Today —

func TestServerWorktimeView_Today_EmptyDay_Weekday_8hTarget(t *testing.T) {
	t.Parallel()
	// 2026-04-29 was a Wednesday.
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	v := mkView(fakeSessionsReader{}, fakeActiveReader{}, now)

	day, err := v.Today("u1")
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
	// 2026-05-02 was a Saturday.
	now := time.Date(2026, 5, 2, 11, 0, 0, 0, time.UTC)
	v := mkView(fakeSessionsReader{}, fakeActiveReader{}, now)

	day, err := v.Today("u1")
	if err != nil {
		t.Fatalf("Today: %v", err)
	}
	if day.Target != 0 {
		t.Errorf("weekend target: got %v, want 0", day.Target)
	}
}

func TestServerWorktimeView_Today_TwoSessionsPlusActive(t *testing.T) {
	t.Parallel()
	userID := "u-today2"
	projectID := "p-swv"

	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	sessions := fakeSessionsReader{sessions: []domain.Session{
		makeSession(userID, projectID, time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC), 2*time.Hour),
		makeSession(userID, projectID, time.Date(2026, 4, 29, 11, 30, 0, 0, time.UTC), 90*time.Minute),
		// Yesterday — must NOT appear in Today.
		makeSession(userID, projectID, time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC), 8*time.Hour),
	}}
	startedAt := time.Date(2026, 4, 29, 13, 30, 0, 0, time.UTC)
	active := fakeActiveReader{rows: []domain.ActiveSession{
		{UserID: userID, ProjectID: projectID, StartedAt: startedAt},
	}}

	v := mkView(sessions, active, now)
	day, err := v.Today(userID)
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
	userID := "u-week1"
	projectID := "p-week"

	// Wednesday 2026-04-29: week is Mon 27 - Sun 03/May.
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	sessions := fakeSessionsReader{sessions: []domain.Session{
		makeSession(userID, projectID, time.Date(2026, 4, 27, 9, 0, 0, 0, time.UTC), 4*time.Hour),
		makeSession(userID, projectID, time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC), 2*time.Hour),
	}}

	v := mkView(sessions, fakeActiveReader{}, now)
	week, err := v.Week(userID)
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

func TestServerWorktimeView_Week_ActiveSessionOnTodayRow(t *testing.T) {
	t.Parallel()
	userID := "u-week-active"
	projectID := "p-week-active"

	// Wednesday 2026-04-29: week is Mon 27 - Sun 03/May.
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	sessions := fakeSessionsReader{sessions: []domain.Session{
		makeSession(userID, projectID, time.Date(2026, 4, 27, 9, 0, 0, 0, time.UTC), 4*time.Hour),
		makeSession(userID, projectID, time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC), 2*time.Hour),
	}}
	startedAt := time.Date(2026, 4, 29, 13, 0, 0, 0, time.UTC)
	active := fakeActiveReader{rows: []domain.ActiveSession{
		{UserID: userID, ProjectID: projectID, StartedAt: startedAt},
	}}

	v := mkView(sessions, active, now)
	week, err := v.Week(userID)
	if err != nil {
		t.Fatalf("Week: %v", err)
	}

	var todayRows int
	for _, wd := range week {
		if wd.IsToday {
			todayRows++
			if wd.Active == nil {
				t.Fatal("today row: Active is nil, want pointer to active session start")
			}
			got := wd.Active.Truncate(time.Second)
			want := startedAt.Truncate(time.Second)
			if !got.Equal(want) {
				t.Errorf("today row Active: got %v, want %v", got, want)
			}
		} else if wd.Active != nil {
			t.Errorf("non-today row %v has Active=%v, want nil", wd.Date, *wd.Active)
		}
	}
	if todayRows != 1 {
		t.Errorf("want exactly 1 IsToday row, got %d", todayRows)
	}
}

func TestServerWorktimeView_Week_ShowWeekendKeepsSatSun(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)

	v := mkView(fakeSessionsReader{}, fakeActiveReader{}, now)
	v.ShowWeekend = true

	week, err := v.Week("u1")
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
	userID := "u-hist1"
	projectID := "p-hist"

	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	base := time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC)

	var rows []domain.Session
	for i, off := range []int{-1, -5, -30} {
		start := base.AddDate(0, 0, off)
		rows = append(rows, makeSession(userID, projectID, start, time.Duration(i+1)*time.Hour))
	}
	// One day outside the 60-day window.
	rows = append(rows, makeSession(userID, projectID, base.AddDate(0, 0, -90), time.Hour))

	v := mkView(fakeSessionsReader{sessions: rows}, fakeActiveReader{}, now)
	hist, err := v.History(userID)
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
	userID := "u-range1"
	projectID := "p-range"

	base := time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC)
	var rows []domain.Session
	for i := -2; i <= 2; i++ {
		rows = append(rows, makeSession(userID, projectID, base.AddDate(0, 0, i), time.Hour))
	}

	v := mkView(fakeSessionsReader{sessions: rows}, fakeActiveReader{}, base)
	got, err := v.Range(userID, base.AddDate(0, 0, -1), base.AddDate(0, 0, 1))
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
	uA := "u-isoA"
	uB := "u-isoB"
	pA := "p-isoA"
	pB := "p-isoB"

	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	sessions := fakeSessionsReader{sessions: []domain.Session{
		makeSession(uA, pA, time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC), 3*time.Hour),
		makeSession(uB, pB, time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC), 5*time.Hour),
	}}
	active := fakeActiveReader{rows: []domain.ActiveSession{
		{UserID: uB, ProjectID: pB, StartedAt: time.Date(2026, 4, 29, 13, 0, 0, 0, time.UTC)},
	}}

	v := mkView(sessions, active, now)
	dayA, err := v.Today(uA)
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
		if s.UserID != uA {
			t.Errorf("uA.Today returned row with userID %q", s.UserID)
		}
	}

	histA, err := v.History(uA)
	if err != nil {
		t.Fatalf("History uA: %v", err)
	}
	for _, rec := range histA {
		for _, s := range rec.Sessions {
			if s.UserID != uA {
				t.Errorf("uA.History returned row with userID %q", s.UserID)
			}
		}
	}
}
