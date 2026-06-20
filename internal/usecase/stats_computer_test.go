package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeSessionStore is an in-memory SessionStore for StatsComputer tests.
// (testutil.FakeSessionStore is in a different package; this one is local to
// usecase_test and is intentionally distinct.)
type fakeSessionStore struct{ list []domain.WorkSession }

func (f fakeSessionStore) Create(context.Context, domain.WorkSession) (domain.WorkSession, error) {
	return domain.WorkSession{}, nil
}
func (f fakeSessionStore) Running(context.Context, string) (domain.WorkSession, bool, error) {
	return domain.WorkSession{}, false, nil
}
func (f fakeSessionStore) Stop(context.Context, string, string, *string, time.Time) (domain.WorkSession, error) {
	return domain.WorkSession{}, nil
}
func (f fakeSessionStore) List(_ context.Context, _ string, since time.Time) ([]domain.WorkSession, error) {
	var out []domain.WorkSession
	for _, s := range f.list {
		if !s.Start.Before(since) {
			out = append(out, s)
		}
	}
	return out, nil
}
func (f fakeSessionStore) Update(context.Context, string, string, *string, string, string, time.Time, *time.Time) (domain.WorkSession, error) {
	return domain.WorkSession{}, nil
}
func (f fakeSessionStore) Delete(context.Context, string, string) error {
	return nil
}
func (f fakeSessionStore) ListRange(_ context.Context, _ string, since, until time.Time) ([]domain.WorkSession, error) {
	var out []domain.WorkSession
	for _, s := range f.list {
		if !s.Start.Before(since) && s.Start.Before(until) {
			out = append(out, s)
		}
	}
	return out, nil
}

// fakeStatsSettings is a settings fake that carries full Settings (including
// DefaultTargetMin). Named differently from dayoffs_test.go's fakeSettings
// (which only holds a Bundesland string).
type fakeStatsSettings struct{ s domain.Settings }

func (f fakeStatsSettings) Get(context.Context, string) (domain.Settings, error) { return f.s, nil }
func (f fakeStatsSettings) SetBundesland(context.Context, string, string) error  { return nil }
func (f fakeStatsSettings) SetTargetConfig(context.Context, string, int, map[time.Weekday]int) error {
	return nil
}

// fakeDayOffStore is a trivial no-op DayOffStore (no manual day-offs).
// Named differently from dayoffs_test.go's fakeDayOffs.
type fakeDayOffStore struct{}

func (fakeDayOffStore) Add(context.Context, string, domain.DayOff) error { return nil }
func (fakeDayOffStore) Delete(context.Context, string, time.Time) error  { return nil }
func (fakeDayOffStore) ListRange(context.Context, string, time.Time, time.Time) ([]domain.DayOff, error) {
	return nil, nil
}

// ptr returns a pointer to t. Not in usecase_test yet (ptr in domain_test is a
// different package).
func ptr(t time.Time) *time.Time { return &t }

// fixedClock is already declared in ics_settings_test.go (same package);
// do NOT redeclare it here.

func newComputer(sessions []domain.WorkSession, set domain.Settings) usecase.StatsComputer {
	return usecase.StatsComputer{
		Sessions: fakeSessionStore{list: sessions},
		Settings: fakeStatsSettings{s: set},
		DayOffs:  usecase.ListDayOffs{Store: fakeDayOffStore{}, Settings: fakeStatsSettings{s: set}, Loc: time.UTC},
		Clock:    fixedClock{t: time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)}, // Monday
		Loc:      time.UTC,
	}
}

func TestStatsComputer_TodaySaldo(t *testing.T) {
	set := domain.Settings{Bundesland: "NW", DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}}
	sessions := []domain.WorkSession{
		{ID: "a", Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC), Stop: ptr(time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC))},
	}
	c := newComputer(sessions, set)
	sum, err := c.Today(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Logged != 2*time.Hour {
		t.Errorf("logged: got %v want 2h", sum.Logged)
	}
	if sum.Target != 8*time.Hour {
		t.Errorf("target: got %v want 8h", sum.Target)
	}
	if sum.Saldo != -6*time.Hour {
		t.Errorf("saldo: got %v want -6h", sum.Saldo)
	}
}

func TestStatsComputer_WeekHasSevenDays(t *testing.T) {
	set := domain.Settings{Bundesland: "NW", DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}}
	c := newComputer(nil, set)
	wk, err := c.Week(context.Background(), "u1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(wk) != 7 {
		t.Fatalf("want 7 days, got %d", len(wk))
	}
	if wk[0].Date.Weekday() != time.Monday {
		t.Errorf("week starts Monday, got %v", wk[0].Date.Weekday())
	}
}

func TestStatsComputer_RangeInvalid(t *testing.T) {
	c := newComputer(nil, domain.Settings{DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}})
	if _, err := c.RangeStats(context.Background(), "u1", "year"); err == nil {
		t.Errorf("want error for invalid range")
	}
}

func TestStatsComputer_Burndown(t *testing.T) {
	set := domain.Settings{Bundesland: "NW", DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}}
	// Seed two sessions in June 2026 (current month per fixedClock).
	stop1 := time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC)
	stop2 := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	sessions := []domain.WorkSession{
		{ID: "a", OwnerID: "u1", Start: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Stop: &stop1},
		{ID: "b", OwnerID: "u1", Start: time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC), Stop: &stop2},
	}
	c := newComputer(sessions, set)
	rep, err := c.Burndown(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	// 2h + 3h = 5h total logged.
	wantTotal := 5 * time.Hour
	if rep.Total != wantTotal {
		t.Errorf("burndown.Total: want %v, got %v", wantTotal, rep.Total)
	}
	if rep.WorkdaysAll <= 0 {
		t.Errorf("burndown.WorkdaysAll should be positive, got %d", rep.WorkdaysAll)
	}
	if rep.WorkdaysDue <= 0 {
		t.Errorf("burndown.WorkdaysDue should be positive, got %d", rep.WorkdaysDue)
	}
}

func TestStatsComputer_WeekWithRunningSession(t *testing.T) {
	set := domain.Settings{Bundesland: "NW", DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}}
	// A session that started today (2026-06-15 = Monday per fixedClock) and has not stopped.
	sessions := []domain.WorkSession{
		{ID: "run", OwnerID: "u1", Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC), Stop: nil},
	}
	c := newComputer(sessions, set)
	wk, err := c.Week(context.Background(), "u1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(wk) != 7 {
		t.Fatalf("want 7 days, got %d", len(wk))
	}
	// Today (Monday) is wk[0]; it should have some logged time from the running session.
	if wk[0].Logged <= 0 {
		t.Errorf("today's logged time should be > 0 for running session, got %v", wk[0].Logged)
	}
	if !wk[0].IsToday {
		t.Errorf("wk[0] should be IsToday=true")
	}
}

func TestStatsComputer_RangeStats_Month(t *testing.T) {
	set := domain.Settings{Bundesland: "NW", DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}}
	stop1 := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	stop2 := time.Date(2026, 6, 10, 11, 0, 0, 0, time.UTC)
	sessions := []domain.WorkSession{
		{ID: "c", OwnerID: "u1", Start: time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC), Stop: &stop1},
		{ID: "d", OwnerID: "u1", Start: time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC), Stop: &stop2},
	}
	c := newComputer(sessions, set)
	st, err := c.RangeStats(context.Background(), "u1", "month")
	if err != nil {
		t.Fatal(err)
	}
	// 3h + 2h = 5h total.
	if st.Total != 5*time.Hour {
		t.Errorf("RangeStats(month).Total: want 5h, got %v", st.Total)
	}
	// DaysWithSessions = 2 (one per day).
	if st.DaysWithSessions != 2 {
		t.Errorf("RangeStats(month).DaysWithSessions: want 2, got %d", st.DaysWithSessions)
	}
	// Workdays should be positive (June has many workdays).
	if st.Workdays <= 0 {
		t.Errorf("RangeStats(month).Workdays should be positive, got %d", st.Workdays)
	}
}

func TestStatsComputer_Today_Running(t *testing.T) {
	set := domain.Settings{Bundesland: "NW", DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}}
	// A running session (no Stop) started at 08:00 today; clock is 14:00 → 6h logged.
	sessions := []domain.WorkSession{
		{ID: "r", OwnerID: "u1", Start: time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC), Stop: nil},
	}
	c := newComputer(sessions, set)
	sum, err := c.Today(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Running {
		t.Error("expected Running=true")
	}
	if sum.Logged != 6*time.Hour {
		t.Errorf("logged: want 6h, got %v", sum.Logged)
	}
}

func TestSetTargetConfig_NegativeWeekday(t *testing.T) {
	// SetTargetConfig.Execute should reject a negative weekday override.
	uc := usecase.SetTargetConfig{Settings: fakeStatsSettings{}}
	err := uc.Execute(context.Background(), "u1", 480, map[time.Weekday]int{time.Friday: -1})
	if err == nil {
		t.Error("want error for negative weekday override, got nil")
	}
}

func TestSetTargetConfig_NegativeDefault(t *testing.T) {
	uc := usecase.SetTargetConfig{Settings: fakeStatsSettings{}}
	err := uc.Execute(context.Background(), "u1", -1, map[time.Weekday]int{})
	if err == nil {
		t.Error("want error for negative defaultMin, got nil")
	}
}

func TestSetTargetConfig_HappyPath(t *testing.T) {
	settings := fakeStatsSettings{s: domain.Settings{DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}}}
	uc := usecase.SetTargetConfig{Settings: settings}
	err := uc.Execute(context.Background(), "u1", 360, map[time.Weekday]int{time.Friday: 240})
	if err != nil {
		t.Errorf("SetTargetConfig.Execute: unexpected error: %v", err)
	}
}

func TestStatsComputer_isoMondayLocal_Sunday(t *testing.T) {
	// 2026-06-14 is a Sunday; its ISO Monday should be 2026-06-08.
	set := domain.Settings{Bundesland: "NW", DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}}
	c := usecase.StatsComputer{
		Sessions: fakeSessionStore{},
		Settings: fakeStatsSettings{s: set},
		DayOffs:  usecase.ListDayOffs{Store: fakeDayOffStore{}, Settings: fakeStatsSettings{s: set}, Loc: time.UTC},
		Clock:    fixedClock{t: time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)}, // Sunday
		Loc:      time.UTC,
	}
	wk, err := c.Week(context.Background(), "u1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(wk) != 7 {
		t.Fatalf("want 7 days, got %d", len(wk))
	}
	// Monday of the week containing Sunday 2026-06-14 is 2026-06-08.
	if wk[0].Date.Weekday() != time.Monday {
		t.Errorf("first day should be Monday, got %v", wk[0].Date.Weekday())
	}
	wantMon := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	if !wk[0].Date.Equal(wantMon) {
		t.Errorf("week[0].Date: want %v, got %v", wantMon, wk[0].Date)
	}
}
