package worktime_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

// useEmptyDayOffs points the dayoffs cache at a non-existent file for the
// duration of the test, so weekday-only assertions don't get derailed by an
// installed holiday list. t.Parallel is incompatible with t.Setenv, so callers
// must not use both.
func useEmptyDayOffs(t *testing.T) {
	t.Helper()
	t.Setenv("WORKTIME_DAYOFFS_FILE", filepath.Join(t.TempDir(), "empty.tsv"))
	worktime.ResetCachesForTesting()
	t.Cleanup(worktime.ResetCachesForTesting)
}

func mkRec(date time.Time, total, target time.Duration) worktime.DayRecord {
	return worktime.DayRecord{Date: date, Total: total, Target: target}
}

func TestAggregate_Empty(t *testing.T) {
	useEmptyDayOffs(t)
	st := worktime.Aggregate(nil)
	if st.Days != 0 || st.Total != 0 || st.Streak != 0 {
		t.Errorf("empty stats = %+v, want zero", st)
	}
}

func TestAggregate_StreakWeekdaysOnly(t *testing.T) {
	useEmptyDayOffs(t)
	target := 8 * time.Hour
	// Mon-Fri of 2026-04-27..05-01: all hit target. Sat/Sun absent.
	records := []worktime.DayRecord{
		mkRec(time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local), target, target), // Mon
		mkRec(time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local), target, target), // Tue
		mkRec(time.Date(2026, 4, 29, 0, 0, 0, 0, time.Local), target, target), // Wed
		mkRec(time.Date(2026, 4, 30, 0, 0, 0, 0, time.Local), target, target), // Thu
		mkRec(time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local), target, target),  // Fri
	}
	st := worktime.Aggregate(records)
	if st.Streak != 5 {
		t.Errorf("Streak = %d, want 5", st.Streak)
	}
	if st.Hits != 5 {
		t.Errorf("Hits = %d, want 5", st.Hits)
	}
	if st.BestStreak != 5 {
		t.Errorf("BestStreak = %d, want 5", st.BestStreak)
	}
	if st.Workdays != 5 {
		t.Errorf("Workdays = %d, want 5", st.Workdays)
	}
}

func TestAggregate_StreakBreaks(t *testing.T) {
	useEmptyDayOffs(t)
	target := 8 * time.Hour
	records := []worktime.DayRecord{
		mkRec(time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local), target, target),
		mkRec(time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local), target/2, target), // miss
		mkRec(time.Date(2026, 4, 29, 0, 0, 0, 0, time.Local), target, target),
	}
	st := worktime.Aggregate(records)
	if st.Streak != 1 {
		t.Errorf("Streak = %d, want 1 (only newest)", st.Streak)
	}
	if st.BestStreak != 1 {
		t.Errorf("BestStreak = %d, want 1", st.BestStreak)
	}
}

func TestAggregate_AvgMaxMin(t *testing.T) {
	useEmptyDayOffs(t)
	records := []worktime.DayRecord{
		mkRec(time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local), 4*time.Hour, 8*time.Hour),
		mkRec(time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local), 8*time.Hour, 8*time.Hour),
		mkRec(time.Date(2026, 4, 29, 0, 0, 0, 0, time.Local), 12*time.Hour, 8*time.Hour),
	}
	st := worktime.Aggregate(records)
	if st.Total != 24*time.Hour {
		t.Errorf("Total = %v, want 24h", st.Total)
	}
	if st.Avg != 8*time.Hour {
		t.Errorf("Avg = %v, want 8h", st.Avg)
	}
	if st.Max != 12*time.Hour {
		t.Errorf("Max = %v, want 12h", st.Max)
	}
	if st.Min != 4*time.Hour {
		t.Errorf("Min = %v, want 4h", st.Min)
	}
}

func TestAggregate_HolidayExcludedFromWorkdays(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WORKTIME_DAYOFFS_FILE", filepath.Join(dir, "dayoffs.tsv"))
	worktime.ResetCachesForTesting()
	t.Cleanup(worktime.ResetCachesForTesting)

	holiday := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local) // Friday → mark as holiday
	if err := worktime.AddDayOff(holiday, worktime.KindHoliday, "Tag der Arbeit"); err != nil {
		t.Fatal(err)
	}
	target := 8 * time.Hour
	records := []worktime.DayRecord{
		mkRec(time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local), target, target),
		mkRec(time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local), target, target),
		mkRec(time.Date(2026, 4, 29, 0, 0, 0, 0, time.Local), target, target),
		mkRec(time.Date(2026, 4, 30, 0, 0, 0, 0, time.Local), target, target),
		// 2026-05-01 is a holiday → excluded from workday count.
	}
	st := worktime.Aggregate(records)
	if st.Workdays != 4 {
		t.Errorf("Workdays = %d, want 4 (Fri excluded as holiday)", st.Workdays)
	}
	if st.Hits != 4 {
		t.Errorf("Hits = %d, want 4", st.Hits)
	}
}

func TestAggregate_ByTag(t *testing.T) {
	useEmptyDayOffs(t)
	rec := worktime.DayRecord{
		Date:   time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local),
		Target: 8 * time.Hour,
		Total:  6 * time.Hour,
		Sessions: []worktime.Session{
			{Elapsed: 2 * time.Hour, Tag: "deep"},
			{Elapsed: 3 * time.Hour, Tag: "deep"},
			{Elapsed: 1 * time.Hour, Tag: "meeting"},
		},
	}
	st := worktime.Aggregate([]worktime.DayRecord{rec})
	if got := st.ByTag["deep"]; got != 5*time.Hour {
		t.Errorf("ByTag[deep] = %v, want 5h", got)
	}
	if got := st.ByTag["meeting"]; got != time.Hour {
		t.Errorf("ByTag[meeting] = %v, want 1h", got)
	}
	tops := st.TopTags(0)
	if len(tops) != 2 || tops[0].Tag != "deep" {
		t.Errorf("TopTags = %v, want deep first", tops)
	}
}
