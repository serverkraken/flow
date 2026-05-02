package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func mkReader(now time.Time, sessions []domain.Session, opts ...readerOpt) *usecase.WorktimeReader {
	cfg := &usecase.TargetResolver{
		Config:        &testutil.FakeConfigReader{Cfg: domain.Config{DefaultTarget: 8 * time.Hour}},
		DayOffs:       testutil.NewFakeDayOffStore(),
		DefaultTarget: 8 * time.Hour,
	}
	r := &usecase.WorktimeReader{
		Sessions: &testutil.FakeSessionStore{Sessions: sessions},
		State:    &testutil.FakeActiveSessionStore{},
		Targets:  cfg,
		Clock:    &testutil.FixedClock{T: now},
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

type readerOpt func(*usecase.WorktimeReader)

func withActive(t time.Time) readerOpt {
	return func(r *usecase.WorktimeReader) {
		r.State = &testutil.FakeActiveSessionStore{Active: &t}
	}
}

func withPause(t time.Time) readerOpt {
	return func(r *usecase.WorktimeReader) {
		r.State = &testutil.FakeActiveSessionStore{Pause: &t}
	}
}

func withShowWeekend() readerOpt {
	return func(r *usecase.WorktimeReader) { r.ShowWeekend = true }
}

func sessAt(date string, h, m int, dur time.Duration) domain.Session {
	d, _ := time.ParseInLocation("2006-01-02", date, time.Local)
	start := d.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
	return domain.Session{Date: d, Start: start, Stop: start.Add(dur), Elapsed: dur}
}

// — Today —

func TestWorktimeReader_Today_IdleEmpty(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	r := mkReader(now, nil)
	day, err := r.Today()
	if err != nil {
		t.Fatal(err)
	}
	if day.IsRunning() || day.IsPaused() {
		t.Errorf("idle day should not be running or paused")
	}
	if day.Target != 8*time.Hour {
		t.Errorf("Target = %v, want 8h", day.Target)
	}
}

func TestWorktimeReader_Today_AggregatesTodayOnly(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	sessions := []domain.Session{
		sessAt("2026-04-29", 9, 0, 2*time.Hour),
		sessAt("2026-04-29", 11, 30, 90*time.Minute),
		sessAt("2026-04-28", 9, 0, 8*time.Hour), // yesterday — must be filtered out
	}
	r := mkReader(now, sessions)
	day, err := r.Today()
	if err != nil {
		t.Fatal(err)
	}
	if len(day.Sessions) != 2 {
		t.Errorf("expected 2 sessions today, got %d", len(day.Sessions))
	}
	if day.Logged != 3*time.Hour+30*time.Minute {
		t.Errorf("Logged = %v, want 3h30m", day.Logged)
	}
}

func TestWorktimeReader_Today_ActiveBeatsPause(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	active := now.Add(-time.Hour)
	pause := now.Add(-2 * time.Hour)
	r := mkReader(now, nil, func(r *usecase.WorktimeReader) {
		r.State = &testutil.FakeActiveSessionStore{Active: &active, Pause: &pause}
	})
	day, _ := r.Today()
	if !day.IsRunning() {
		t.Error("active should win")
	}
	if day.IsPaused() {
		t.Error("active should suppress pause")
	}
}

func TestWorktimeReader_Today_PauseAlone(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	pause := now.Add(-30 * time.Minute)
	r := mkReader(now, nil, withPause(pause))
	day, _ := r.Today()
	if !day.IsPaused() {
		t.Error("pause-only day should be paused")
	}
}

func TestWorktimeReader_Today_PropagatesSessionStoreErr(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	r := mkReader(now, nil)
	r.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	if _, err := r.Today(); err == nil {
		t.Error("expected error from store")
	}
}

// — Week —

func TestWorktimeReader_Week_MonToFriHasFiveRowsByDefault(t *testing.T) {
	wed := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local) // Wednesday
	r := mkReader(wed, []domain.Session{
		sessAt("2026-04-27", 9, 0, 2*time.Hour),
	})
	week, err := r.Week()
	if err != nil {
		t.Fatal(err)
	}
	// Mon..Fri = 5 rows. Sat/Sun dropped (no sessions, not today, ShowWeekend=false).
	if len(week) != 5 {
		t.Errorf("expected 5 rows, got %d", len(week))
	}
	// First row should be Monday.
	if week[0].Date.Weekday() != time.Monday {
		t.Errorf("first row weekday = %v, want Monday", week[0].Date.Weekday())
	}
	// Today flag set on Wed.
	wedRow := week[2]
	if !wedRow.IsToday {
		t.Errorf("Wednesday should be IsToday")
	}
}

func TestWorktimeReader_Week_ShowWeekendIncludesSatSun(t *testing.T) {
	wed := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local)
	r := mkReader(wed, nil, withShowWeekend())
	week, _ := r.Week()
	if len(week) != 7 {
		t.Errorf("expected 7 rows with ShowWeekend, got %d", len(week))
	}
}

func TestWorktimeReader_Week_SatWithSessionRendered(t *testing.T) {
	wed := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local)
	r := mkReader(wed, []domain.Session{
		sessAt("2026-05-02", 10, 0, time.Hour), // Saturday
	})
	// Wait — Saturday 2026-05-02 is in the same week? Mon=04-27, Sun=05-03,
	// so Sat=05-02 is in the same week. Yes.
	week, _ := r.Week()
	// Mon..Fri (5) + Sat (has session). Sun has none and isn't today, so dropped.
	if len(week) != 6 {
		t.Errorf("expected 6 rows (Sat included), got %d: %+v", len(week), week)
	}
}

func TestWorktimeReader_Week_TodayIsSundayDecorrelatesActiveCorrectly(t *testing.T) {
	sun := time.Date(2026, 5, 3, 12, 0, 0, 0, time.Local) // Sunday — wd==0 → 7
	active := sun.Add(-time.Hour)
	r := mkReader(sun, nil, withActive(active))
	week, err := r.Week()
	if err != nil {
		t.Fatal(err)
	}
	// Sunday is today, so it must be included even with no sessions.
	if len(week) == 0 {
		t.Fatal("expected at least Sunday row")
	}
	last := week[len(week)-1]
	if !last.IsToday {
		t.Errorf("last row should be today (Sunday)")
	}
	if last.Active == nil {
		t.Errorf("Sunday today should carry the active marker")
	}
}

func TestWorktimeReader_Week_PropagatesError(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local)
	r := mkReader(now, nil)
	r.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	if _, err := r.Week(); err == nil {
		t.Error("expected error")
	}
}

// — History —

func TestWorktimeReader_History_NewestFirst(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	r := mkReader(now, []domain.Session{
		sessAt("2026-04-27", 9, 0, time.Hour),
		sessAt("2026-04-29", 9, 0, time.Hour),
		sessAt("2026-04-28", 9, 0, time.Hour),
	})
	hist, err := r.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 3 {
		t.Fatalf("expected 3 day records, got %d", len(hist))
	}
	wantOrder := []string{"2026-04-29", "2026-04-28", "2026-04-27"}
	for i, want := range wantOrder {
		got := hist[i].Date.Format("2006-01-02")
		if got != want {
			t.Errorf("position %d: got %s, want %s", i, got, want)
		}
	}
}

func TestWorktimeReader_History_AggregatesPerDay(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	r := mkReader(now, []domain.Session{
		sessAt("2026-04-29", 9, 0, 2*time.Hour),
		sessAt("2026-04-29", 12, 0, time.Hour),
	})
	hist, _ := r.History()
	if len(hist) != 1 {
		t.Fatalf("expected 1 day record, got %d", len(hist))
	}
	if hist[0].Total != 3*time.Hour {
		t.Errorf("Total = %v, want 3h", hist[0].Total)
	}
	if len(hist[0].Sessions) != 2 {
		t.Errorf("Sessions count = %d, want 2", len(hist[0].Sessions))
	}
}

func TestWorktimeReader_History_PropagatesError(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	r := mkReader(now, nil)
	r.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	if _, err := r.History(); err == nil {
		t.Error("expected error")
	}
}

// — Range —

func TestWorktimeReader_Range_EmptyReturnsAll(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	sessions := []domain.Session{
		sessAt("2026-04-27", 9, 0, time.Hour),
		sessAt("2026-04-30", 9, 0, time.Hour),
	}
	r := mkReader(now, sessions)
	got, err := r.Range(domain.Range{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("empty range should return all, got %d", len(got))
	}
}

func TestWorktimeReader_Range_FiltersByDate(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	sessions := []domain.Session{
		sessAt("2026-04-27", 9, 0, time.Hour),
		sessAt("2026-04-29", 9, 0, time.Hour),
		sessAt("2026-04-30", 9, 0, time.Hour),
	}
	r := mkReader(now, sessions)
	rng := domain.Range{
		From: time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local),
		To:   time.Date(2026, 4, 30, 0, 0, 0, 0, time.Local),
	}
	got, _ := r.Range(rng)
	if len(got) != 1 || got[0].Date.Day() != 29 {
		t.Errorf("range should keep only 04-29, got %+v", got)
	}
}

// — SessionsOverlap —

func TestWorktimeReader_SessionsOverlap(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-30", time.Local)
	sessions := []domain.Session{
		// 09:00–11:00
		{Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)},
		// 13:00–15:00
		{Date: d, Start: d.Add(13 * time.Hour), Stop: d.Add(15 * time.Hour)},
	}
	r := mkReader(now, sessions)

	t.Run("non-overlapping span", func(t *testing.T) {
		hit, _, _ := r.SessionsOverlap(d, d.Add(11*time.Hour+30*time.Minute), d.Add(12*time.Hour), -1)
		if hit {
			t.Error("11:30–12:00 should not overlap")
		}
	})
	t.Run("overlaps existing", func(t *testing.T) {
		hit, conflict, _ := r.SessionsOverlap(d, d.Add(10*time.Hour), d.Add(10*time.Hour+30*time.Minute), -1)
		if !hit {
			t.Error("10:00–10:30 should overlap 09:00–11:00")
		}
		if conflict == nil || conflict.Start.Hour() != 9 {
			t.Errorf("conflict pointer wrong: %+v", conflict)
		}
	})
	t.Run("excludes self", func(t *testing.T) {
		// Session 0 is 09:00–11:00. Editing it to 10:00–11:00 must not
		// flag itself.
		hit, _, _ := r.SessionsOverlap(d, d.Add(10*time.Hour), d.Add(11*time.Hour), 0)
		if hit {
			t.Error("editing session 0 should not flag itself")
		}
	})
	t.Run("different date is ignored", func(t *testing.T) {
		other := d.AddDate(0, 0, 1)
		hit, _, _ := r.SessionsOverlap(other, other.Add(10*time.Hour), other.Add(11*time.Hour), -1)
		if hit {
			t.Error("other date should not overlap")
		}
	})
	t.Run("propagates error", func(t *testing.T) {
		r.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
		if _, _, err := r.SessionsOverlap(d, d, d, -1); err == nil {
			t.Error("expected error")
		}
	})
}
