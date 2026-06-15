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
