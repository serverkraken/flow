package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func at(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.Local)
}

func rec(d time.Time, total, target time.Duration, sessions ...domain.Session) domain.DayRecord {
	return domain.DayRecord{Date: d, Total: total, Target: target, Sessions: sessions}
}

func sess(tag string, elapsed time.Duration) domain.Session {
	return domain.Session{Tag: tag, Elapsed: elapsed}
}

// allWorkdays treats every input date as a workday. Lets tests focus on the
// aggregation logic without juggling weekday/dayoff inputs.
func allWorkdays(time.Time) bool { return true }

func TestAggregate_Empty(t *testing.T) {
	got := domain.Aggregate(nil, allWorkdays, nil)
	if got.Days != 0 || got.Total != 0 {
		t.Errorf("expected zero-state stats, got %+v", got)
	}
	if got.ByTag == nil || got.CountByTag == nil {
		t.Error("expected non-nil empty maps")
	}
}

func TestAggregate_SingleHit(t *testing.T) {
	r := rec(at(2026, 4, 29), 8*time.Hour, 8*time.Hour, sess("deep", 8*time.Hour))
	st := domain.Aggregate([]domain.DayRecord{r}, allWorkdays, nil)

	if st.Days != 1 || st.Workdays != 1 || st.Hits != 1 {
		t.Errorf("Days/Workdays/Hits = %d/%d/%d", st.Days, st.Workdays, st.Hits)
	}
	if st.Streak != 1 || st.BestStreak != 1 {
		t.Errorf("Streak/BestStreak = %d/%d", st.Streak, st.BestStreak)
	}
	if st.Total != 8*time.Hour || st.Avg != 8*time.Hour {
		t.Errorf("Total/Avg = %v/%v", st.Total, st.Avg)
	}
	if st.ByTag["deep"] != 8*time.Hour {
		t.Errorf("ByTag[deep] = %v", st.ByTag["deep"])
	}
}

func TestAggregate_MissResetsStreak(t *testing.T) {
	// Two workdays: hit, then miss. BestStreak should be 1 (high-water from
	// the hit), Streak 0 (newest is a miss → backward walk breaks immediately).
	r1 := rec(at(2026, 4, 28), 8*time.Hour, 8*time.Hour)
	r2 := rec(at(2026, 4, 29), 4*time.Hour, 8*time.Hour)
	st := domain.Aggregate([]domain.DayRecord{r1, r2}, allWorkdays, nil)
	if st.BestStreak != 1 {
		t.Errorf("BestStreak = %d, want 1", st.BestStreak)
	}
	if st.Streak != 0 {
		t.Errorf("Streak = %d, want 0", st.Streak)
	}
	if st.Hits != 1 {
		t.Errorf("Hits = %d, want 1", st.Hits)
	}
}

func TestAggregate_StreakSpansNonWorkday(t *testing.T) {
	// hit, weekend (skipped), hit — backward streak is 2 because non-workdays
	// are transparent.
	wkday := func(d time.Time) bool {
		return d.Weekday() != time.Saturday && d.Weekday() != time.Sunday
	}
	r1 := rec(at(2026, 4, 24), 8*time.Hour, 8*time.Hour) // Fri
	r2 := rec(at(2026, 4, 25), 1*time.Hour, 8*time.Hour) // Sat (non-workday → transparent)
	r3 := rec(at(2026, 4, 27), 8*time.Hour, 8*time.Hour) // Mon
	st := domain.Aggregate([]domain.DayRecord{r1, r2, r3}, wkday, nil)
	if st.Streak != 2 {
		t.Errorf("Streak = %d, want 2", st.Streak)
	}
	if st.BestStreak != 2 {
		t.Errorf("BestStreak = %d, want 2", st.BestStreak)
	}
}

func TestAggregate_MinMaxAcrossRecords(t *testing.T) {
	r1 := rec(at(2026, 4, 27), 4*time.Hour, 8*time.Hour) // smallest
	r2 := rec(at(2026, 4, 28), 8*time.Hour, 8*time.Hour) // largest
	r3 := rec(at(2026, 4, 29), 6*time.Hour, 8*time.Hour) // middle
	st := domain.Aggregate([]domain.DayRecord{r1, r2, r3}, allWorkdays, nil)
	if st.Max != 8*time.Hour || !st.MaxDate.Equal(at(2026, 4, 28)) {
		t.Errorf("Max = %v on %v", st.Max, st.MaxDate)
	}
	if st.Min != 4*time.Hour || !st.MinDate.Equal(at(2026, 4, 27)) {
		t.Errorf("Min = %v on %v", st.Min, st.MinDate)
	}
}

func TestAggregate_OutOfOrderInputIsSorted(t *testing.T) {
	// Newest first input — Aggregate should sort and produce the same
	// streak result as chronological input.
	r1 := rec(at(2026, 4, 29), 8*time.Hour, 8*time.Hour)
	r2 := rec(at(2026, 4, 27), 8*time.Hour, 8*time.Hour)
	r3 := rec(at(2026, 4, 28), 8*time.Hour, 8*time.Hour)
	st := domain.Aggregate([]domain.DayRecord{r1, r2, r3}, allWorkdays, nil)
	if st.Streak != 3 || st.BestStreak != 3 {
		t.Errorf("Streak/BestStreak = %d/%d, want 3/3", st.Streak, st.BestStreak)
	}
}

func TestAggregate_NonWorkdayDoesNotCount(t *testing.T) {
	noWorkdays := func(time.Time) bool { return false }
	r := rec(at(2026, 4, 29), 8*time.Hour, 8*time.Hour, sess("deep", 8*time.Hour))
	st := domain.Aggregate([]domain.DayRecord{r}, noWorkdays, nil)
	if st.Workdays != 0 || st.Hits != 0 || st.Streak != 0 {
		t.Errorf("expected zero work-fields, got Workdays=%d Hits=%d Streak=%d",
			st.Workdays, st.Hits, st.Streak)
	}
	if st.Days != 1 {
		t.Errorf("Days = %d, want 1", st.Days)
	}
}

func TestAggregate_DayOffsListed(t *testing.T) {
	r1 := rec(at(2026, 4, 27), 8*time.Hour, 8*time.Hour)
	r2 := rec(at(2026, 4, 30), 8*time.Hour, 8*time.Hour)
	want := []domain.DayOff{
		{Date: at(2026, 4, 28), Kind: domain.KindHoliday, Label: "Test"},
	}
	listDayOffs := func(from, to time.Time) []domain.DayOff {
		if !from.Equal(at(2026, 4, 27)) || !to.Equal(at(2026, 4, 30)) {
			t.Errorf("listDayOffs called with [%v,%v]", from, to)
		}
		return want
	}
	st := domain.Aggregate([]domain.DayRecord{r1, r2}, allWorkdays, listDayOffs)
	if !reflect.DeepEqual(st.DaysOff, want) {
		t.Errorf("DaysOff = %#v, want %#v", st.DaysOff, want)
	}
}

// TestAggregateRange_EmptyRangeReturnsEmpty pinst die Defaultpfad-
// Garantie (from >= to → leere Stats, kein panic).
func TestAggregateRange_EmptyRangeReturnsEmpty(t *testing.T) {
	st := domain.AggregateRange(
		nil,
		at(2026, 4, 29), at(2026, 4, 29),
		allWorkdays,
		func(time.Time) time.Duration { return 8 * time.Hour },
		nil,
	)
	if st.Days != 0 || st.Workdays != 0 || st.Overtime != 0 {
		t.Errorf("empty range: %+v", st)
	}
}

// TestAggregateRange_UnvisitedWorkdaysCountTowardSaldo unterscheidet
// AggregateRange von Aggregate: ein Workday im Range ohne DayRecord
// trägt -targetFor(d) zum Saldo bei. Bei Aggregate fehlt diese
// Korrektur, weil dort nur die Records selbst gezählt werden.
func TestAggregateRange_UnvisitedWorkdaysCountTowardSaldo(t *testing.T) {
	r := rec(at(2026, 4, 27), 8*time.Hour, 8*time.Hour) // Montag voll
	st := domain.AggregateRange(
		[]domain.DayRecord{r},
		at(2026, 4, 27), at(2026, 4, 30), // Mo-Mi (3 Workdays)
		allWorkdays,
		func(time.Time) time.Duration { return 8 * time.Hour },
		nil,
	)
	if st.Workdays != 3 {
		t.Errorf("Workdays = %d, want 3 (range bestimmt, nicht Records)", st.Workdays)
	}
	if st.Hits != 1 {
		t.Errorf("Hits = %d, want 1 (Mo war voll, Di+Mi unbesetzt)", st.Hits)
	}
	// Saldo: Mo +0h (Soll erfüllt), Di -8h, Mi -8h = -16h
	if st.Overtime != -16*time.Hour {
		t.Errorf("Overtime = %v, want -16h", st.Overtime)
	}
}

// TestAggregateRange_TargetZeroWorkdayCountsAsMiss pinst die isHit-
// Konvention: ein Workday mit Target=0 darf NICHT als Hit zählen
// (auch wenn Total ≥ 0). Pre-round4 Aggregate-Pfad zählte solche
// Tage als Hit (Drift gegenüber AggregateRange) — gemeinsame isHit-
// Funktion in aggregate.go schließt das.
func TestAggregateRange_TargetZeroWorkdayCountsAsMiss(t *testing.T) {
	r := rec(at(2026, 4, 27), 0, 0) // Mo: Total=0, Target=0
	st := domain.AggregateRange(
		[]domain.DayRecord{r},
		at(2026, 4, 27), at(2026, 4, 28),
		allWorkdays,
		func(time.Time) time.Duration { return 0 },
		nil,
	)
	if st.Hits != 0 {
		t.Errorf("Target=0 darf nicht als Hit zählen, got Hits=%d", st.Hits)
	}
}

func TestFilterRecords(t *testing.T) {
	records := []domain.DayRecord{
		{Date: at(2026, 4, 27)},
		{Date: at(2026, 4, 28)},
		{Date: at(2026, 4, 29)},
		{Date: at(2026, 4, 30)},
	}
	got := domain.FilterRecords(records, at(2026, 4, 28), at(2026, 4, 30))
	if len(got) != 2 || !got[0].Date.Equal(at(2026, 4, 28)) || !got[1].Date.Equal(at(2026, 4, 29)) {
		t.Errorf("FilterRecords = %#v", got)
	}
}

func TestBriefBounds(t *testing.T) {
	wed := time.Date(2026, time.April, 29, 14, 30, 0, 0, time.Local) // Wednesday
	sun := time.Date(2026, time.May, 3, 14, 30, 0, 0, time.Local)    // Sunday triggers wd==0 → 7

	t.Run("week from Wednesday", func(t *testing.T) {
		from, to, title := domain.BriefBounds(wed, domain.ReportWeek)
		if !from.Equal(at(2026, 4, 27)) {
			t.Errorf("from = %v", from)
		}
		if !to.Equal(at(2026, 5, 4)) {
			t.Errorf("to = %v", to)
		}
		if title == "" {
			t.Error("empty title")
		}
	})

	t.Run("week from Sunday clamps to that week", func(t *testing.T) {
		from, _, _ := domain.BriefBounds(sun, domain.ReportWeek)
		if !from.Equal(at(2026, 4, 27)) {
			t.Errorf("from = %v, want 2026-04-27", from)
		}
	})

	t.Run("month", func(t *testing.T) {
		from, to, title := domain.BriefBounds(wed, domain.ReportMonth)
		if !from.Equal(at(2026, 4, 1)) || !to.Equal(at(2026, 5, 1)) {
			t.Errorf("month bounds = [%v, %v)", from, to)
		}
		if title == "" {
			t.Error("empty title")
		}
	})
}

func TestPlannedTarget(t *testing.T) {
	// 7 calendar days, 5 workdays, 8h each → 40h. The two non-workdays
	// (sat/sun) are skipped.
	wkday := func(d time.Time) bool {
		return d.Weekday() != time.Saturday && d.Weekday() != time.Sunday
	}
	target := func(time.Time) time.Duration { return 8 * time.Hour }

	got := domain.PlannedTarget(at(2026, 4, 27), at(2026, 5, 4), wkday, target)
	if got != 40*time.Hour {
		t.Errorf("PlannedTarget = %v, want 40h", got)
	}
}

func TestMonthBurndownCompute(t *testing.T) {
	// April 2026: 30 days, with weekends and one holiday on 4/13.
	now := time.Date(2026, time.April, 30, 14, 0, 0, 0, time.Local)
	holiday := at(2026, 4, 13)
	isWorkday := func(d time.Time) bool {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			return false
		}
		return !d.Equal(holiday)
	}
	target := func(time.Time) time.Duration { return 8 * time.Hour }

	t.Run("no records, no active", func(t *testing.T) {
		rep := domain.MonthBurndownCompute(now, nil, nil, isWorkday, target)
		if rep.Total != 0 {
			t.Errorf("Total = %v, want 0", rep.Total)
		}
		if rep.WorkdaysAll == 0 {
			t.Error("WorkdaysAll should reflect April 2026 calendar")
		}
		if rep.Saldo > 0 {
			t.Errorf("Saldo positive without any work logged: %v", rep.Saldo)
		}
		if rep.OnTrack {
			t.Error("OnTrack with negative saldo")
		}
	})

	t.Run("records inside month count", func(t *testing.T) {
		records := []domain.DayRecord{
			{Date: at(2026, 4, 27), Total: 8 * time.Hour},
			{Date: at(2026, 4, 28), Total: 8 * time.Hour},
			{Date: at(2026, 3, 31), Total: 8 * time.Hour}, // outside, ignored
		}
		rep := domain.MonthBurndownCompute(now, records, nil, isWorkday, target)
		if rep.Total != 16*time.Hour {
			t.Errorf("Total = %v, want 16h", rep.Total)
		}
	})

	t.Run("active session in month adds tail", func(t *testing.T) {
		start := time.Date(2026, time.April, 30, 12, 0, 0, 0, time.Local)
		rep := domain.MonthBurndownCompute(now, nil, &start, isWorkday, target)
		if rep.Total != 2*time.Hour {
			t.Errorf("Total = %v, want 2h", rep.Total)
		}
	})

	t.Run("active session crossed midnight clamps to today", func(t *testing.T) {
		// Started yesterday at 22:00; today's slice starts at 00:00.
		start := time.Date(2026, time.April, 29, 22, 0, 0, 0, time.Local)
		rep := domain.MonthBurndownCompute(now, nil, &start, isWorkday, target)
		// today midnight → now (14:00) = 14h
		if rep.Total != 14*time.Hour {
			t.Errorf("Total = %v, want 14h", rep.Total)
		}
	})

	t.Run("active session outside month is ignored", func(t *testing.T) {
		start := time.Date(2026, time.March, 30, 12, 0, 0, 0, time.Local)
		rep := domain.MonthBurndownCompute(now, nil, &start, isWorkday, target)
		if rep.Total != 0 {
			t.Errorf("Total = %v, want 0", rep.Total)
		}
	})
}
