package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newStreakStats wires a StatsComputer over mutable testutil fakes; the default
// FakeUserSettingsStore target is 8h (domain.DefaultDailyTargetMin).
func newStreakStats(t *testing.T, now time.Time) (usecase.StatsComputer, *testutil.FakeSessionStore) {
	t.Helper()
	sessions := testutil.NewFakeSessionStore()
	settings := testutil.NewFakeUserSettingsStore()
	dayoffs := testutil.NewFakeDayOffStore()
	listDayOffs := usecase.ListDayOffs{Store: dayoffs, Settings: settings, Loc: time.UTC}
	return usecase.StatsComputer{
		Sessions: sessions, Settings: settings, DayOffs: listDayOffs,
		Clock: testutil.FakeClock{T: now}, Loc: time.UTC,
	}, sessions
}

// hit8h creates a completed 8h session (09:00-17:00) on the given day.
func hit8h(t *testing.T, s *testutil.FakeSessionStore, owner string, day time.Time) {
	t.Helper()
	start := time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, time.UTC)
	stop := start.Add(8 * time.Hour)
	if _, err := s.Create(context.Background(), domain.WorkSession{
		ID: start.Format("id-2006-01-02"), OwnerID: owner, Start: start, Stop: &stop,
	}); err != nil {
		t.Fatalf("seed hit: %v", err)
	}
}

func TestCurrentStreak_CrossesMonthBoundary(t *testing.T) {
	// "today" = 2026-07-02 (Do). Hits on 06-30 (Di), 07-01 (Mi), 07-02 (Do) →
	// streak 3, spanning the June→July boundary that RangeStats("month") cuts.
	now := time.Date(2026, 7, 2, 18, 0, 0, 0, time.UTC)
	uc, sessions := newStreakStats(t, now)
	for _, d := range []time.Time{
		time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC),
	} {
		stop := d.Add(8 * time.Hour)
		_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: d.Format("id-2006-01-02"), OwnerID: "u1", Start: d, Stop: &stop})
	}
	got, err := uc.CurrentStreak(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got != 3 {
		t.Fatalf("cross-month streak = %d, want 3 (no month-window cut)", got)
	}
}

func TestCurrentStreak_EmptyHistoryIsZero(t *testing.T) {
	now := time.Date(2026, 7, 2, 18, 0, 0, 0, time.UTC)
	uc, _ := newStreakStats(t, now)
	got, err := uc.CurrentStreak(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("empty history streak = %d, want 0", got)
	}
}

func TestCurrentStreak_MissBreaksStreak(t *testing.T) {
	// today (Do 07-02) hit, Mi 07-01 a MISS (only 2h) → streak 1, not 2.
	now := time.Date(2026, 7, 2, 18, 0, 0, 0, time.UTC)
	uc, sessions := newStreakStats(t, now)
	hit8h(t, sessions, "u1", time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC))
	miss := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	missStop := miss.Add(2 * time.Hour)
	_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: "miss", OwnerID: "u1", Start: miss, Stop: &missStop})
	got, err := uc.CurrentStreak(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("miss should break streak → want 1, got %d", got)
	}
}

func TestCurrentStreak_WeekendDoesNotBreak(t *testing.T) {
	// today Mo 07-06 hit, Fr 07-03 hit, Sa/So between are skipped → streak 2.
	now := time.Date(2026, 7, 6, 18, 0, 0, 0, time.UTC)
	uc, sessions := newStreakStats(t, now)
	hit8h(t, sessions, "u1", time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC))
	hit8h(t, sessions, "u1", time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC))
	got, err := uc.CurrentStreak(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("weekend between hits should not break → want 2, got %d", got)
	}
}
