package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func mkComputer(now time.Time, sessions []domain.Session, opts ...readerOpt) *usecase.StatsComputer {
	reader := mkReader(now, sessions, opts...)
	return &usecase.StatsComputer{
		Reader:  reader,
		Targets: reader.Targets,
		DayOffs: reader.Targets.DayOffs,
		State:   reader.State,
	}
}

func TestStatsComputer_Aggregate(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	c := mkComputer(now, nil)
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	records := []domain.DayRecord{
		{Date: mon, Total: 8 * time.Hour, Target: 8 * time.Hour},
	}
	got := c.Aggregate(records)
	if got.Workdays != 1 || got.Hits != 1 {
		t.Errorf("Workdays/Hits = %d/%d, want 1/1", got.Workdays, got.Hits)
	}
}

func TestStatsComputer_CurrentStreak(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local) // Wed
	mon := time.Date(2026, 4, 27, 9, 0, 0, 0, time.Local)
	tue := time.Date(2026, 4, 28, 9, 0, 0, 0, time.Local)
	wed := time.Date(2026, 4, 29, 9, 0, 0, 0, time.Local)
	sessions := []domain.Session{
		{Date: time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local), Start: mon, Stop: mon.Add(8 * time.Hour), Elapsed: 8 * time.Hour},
		{Date: time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local), Start: tue, Stop: tue.Add(8 * time.Hour), Elapsed: 8 * time.Hour},
		{Date: time.Date(2026, 4, 29, 0, 0, 0, 0, time.Local), Start: wed, Stop: wed.Add(8 * time.Hour), Elapsed: 8 * time.Hour},
	}
	c := mkComputer(now, sessions)
	if got := c.CurrentStreak(); got != 3 {
		t.Errorf("CurrentStreak = %d, want 3", got)
	}
}

func TestStatsComputer_CurrentStreak_EmptyHistoryReturnsZero(t *testing.T) {
	c := mkComputer(time.Now(), nil)
	if got := c.CurrentStreak(); got != 0 {
		t.Errorf("empty history: got %d, want 0", got)
	}
}

func TestStatsComputer_CurrentStreak_LoadErrorReturnsZero(t *testing.T) {
	c := mkComputer(time.Now(), nil)
	c.Reader.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	if got := c.CurrentStreak(); got != 0 {
		t.Errorf("load error: got %d, want 0", got)
	}
}

func TestStatsComputer_WeekStats_FiltersToWeek(t *testing.T) {
	wed := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local)
	mon := time.Date(2026, 4, 27, 9, 0, 0, 0, time.Local)
	prevWeek := time.Date(2026, 4, 20, 9, 0, 0, 0, time.Local)
	sessions := []domain.Session{
		{Date: time.Date(2026, 4, 20, 0, 0, 0, 0, time.Local), Start: prevWeek, Stop: prevWeek.Add(8 * time.Hour), Elapsed: 8 * time.Hour},
		{Date: time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local), Start: mon, Stop: mon.Add(8 * time.Hour), Elapsed: 8 * time.Hour},
	}
	c := mkComputer(wed, sessions)
	st, err := c.WeekStats(wed)
	if err != nil {
		t.Fatal(err)
	}
	if st.Days != 1 {
		t.Errorf("WeekStats.Days = %d, want 1 (only this-week 04-27)", st.Days)
	}
}

func TestStatsComputer_WeekStats_FromSundayClampsToCorrectMonday(t *testing.T) {
	// Sunday triggers the wd==0 → 7 branch. The Monday of "this week" is
	// 6 days before, not the next morning.
	sun := time.Date(2026, 5, 3, 12, 0, 0, 0, time.Local)
	mon := time.Date(2026, 4, 27, 9, 0, 0, 0, time.Local) // Monday of same week
	prevSun := time.Date(2026, 4, 26, 9, 0, 0, 0, time.Local)
	sessions := []domain.Session{
		{Date: time.Date(2026, 4, 26, 0, 0, 0, 0, time.Local), Start: prevSun, Stop: prevSun.Add(time.Hour), Elapsed: time.Hour},
		{Date: time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local), Start: mon, Stop: mon.Add(time.Hour), Elapsed: time.Hour},
	}
	c := mkComputer(sun, sessions)
	st, err := c.WeekStats(sun)
	if err != nil {
		t.Fatal(err)
	}
	// Only the 27th (Monday) is in this week's [Mon, next-Mon).
	if st.Days != 1 {
		t.Errorf("expected 1 day in week from Sunday, got %d", st.Days)
	}
}

func TestStatsComputer_MonthStats_FiltersToMonth(t *testing.T) {
	mid := time.Date(2026, 4, 15, 12, 0, 0, 0, time.Local)
	mar := time.Date(2026, 3, 30, 9, 0, 0, 0, time.Local)
	apr := time.Date(2026, 4, 5, 9, 0, 0, 0, time.Local)
	sessions := []domain.Session{
		{Date: time.Date(2026, 3, 30, 0, 0, 0, 0, time.Local), Start: mar, Stop: mar.Add(time.Hour), Elapsed: time.Hour},
		{Date: time.Date(2026, 4, 5, 0, 0, 0, 0, time.Local), Start: apr, Stop: apr.Add(time.Hour), Elapsed: time.Hour},
	}
	c := mkComputer(mid, sessions)
	st, err := c.MonthStats(mid)
	if err != nil {
		t.Fatal(err)
	}
	if st.Days != 1 {
		t.Errorf("MonthStats.Days = %d, want 1 (only April record)", st.Days)
	}
}

func TestStatsComputer_WeekStats_PropagatesError(t *testing.T) {
	c := mkComputer(time.Now(), nil)
	c.Reader.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	if _, err := c.WeekStats(time.Now()); err == nil {
		t.Error("expected error")
	}
}

func TestStatsComputer_MonthStats_PropagatesError(t *testing.T) {
	c := mkComputer(time.Now(), nil)
	c.Reader.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	if _, err := c.MonthStats(time.Now()); err == nil {
		t.Error("expected error")
	}
}

func TestStatsComputer_Burndown_IncludesActiveTail(t *testing.T) {
	now := time.Date(2026, 4, 30, 14, 0, 0, 0, time.Local)
	start := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	c := mkComputer(now, nil, withActive(start))
	rep, err := c.Burndown(now)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Total != 2*time.Hour {
		t.Errorf("Total = %v, want 2h (active tail)", rep.Total)
	}
}

func TestStatsComputer_Burndown_PropagatesError(t *testing.T) {
	c := mkComputer(time.Now(), nil)
	c.Reader.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	if _, err := c.Burndown(time.Now()); err == nil {
		t.Error("expected error")
	}
}
