package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func ptr(t time.Time) *time.Time { return &t }

func TestBuildDayRecords_GroupsAndSumsPerDay(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 6, 15, 14, 0, 0, 0, loc) // Monday
	d1 := time.Date(2026, 6, 14, 0, 0, 0, 0, loc)   // Sunday
	target := func(time.Time) time.Duration { return 8 * time.Hour }

	sessions := []domain.WorkSession{
		{ID: "a", Start: time.Date(2026, 6, 14, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 14, 10, 0, 0, 0, loc)), Tag: "deep"},
		{ID: "b", Start: time.Date(2026, 6, 14, 11, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 14, 11, 30, 0, 0, loc)), Tag: ""},
		{ID: "c", Start: time.Date(2026, 6, 15, 13, 0, 0, 0, loc), Stop: nil, Tag: "meeting"},
	}

	recs := domain.BuildDayRecords(sessions, now, target)
	if len(recs) != 2 {
		t.Fatalf("want 2 day records, got %d", len(recs))
	}
	byDay := map[string]domain.DayRecord{}
	for _, r := range recs {
		byDay[r.Date.Format("2006-01-02")] = r
	}
	sun := byDay[d1.Format("2006-01-02")]
	if sun.Total != 90*time.Minute {
		t.Errorf("sunday total: got %v want 1h30m", sun.Total)
	}
	if sun.Target != 8*time.Hour {
		t.Errorf("sunday target: got %v", sun.Target)
	}
	if len(sun.Sessions) != 2 {
		t.Errorf("sunday sessions: got %d want 2", len(sun.Sessions))
	}
	mon := byDay[now.Format("2006-01-02")]
	if mon.Total != time.Hour {
		t.Errorf("monday live tail: got %v want 1h", mon.Total)
	}
}

func TestBuildDayRecords_Empty(t *testing.T) {
	if r := domain.BuildDayRecords(nil, time.Now(), func(time.Time) time.Duration { return 0 }); len(r) != 0 {
		t.Errorf("want empty, got %d", len(r))
	}
}

// TestBuildDayRecords_GroupsInNowLocation pins the timezone fix: sessions are
// stored as UTC (Postgres timestamptz), but grouping must use now's location.
// A session started 23:30 UTC belongs to the *next* local day in a +2 zone.
func TestBuildDayRecords_GroupsInNowLocation(t *testing.T) {
	cest := time.FixedZone("CEST", 2*60*60) // UTC+2
	// now is Monday 2026-06-15 10:00 local (08:00 UTC).
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, cest)
	target := func(time.Time) time.Duration { return 8 * time.Hour }

	// Session stored in UTC at 2026-06-14 23:30 → 2026-06-15 01:30 local.
	// It must land in the 2026-06-15 (local) bucket, not 2026-06-14 (UTC).
	sessions := []domain.WorkSession{
		{ID: "a",
			Start: time.Date(2026, 6, 14, 23, 30, 0, 0, time.UTC),
			Stop:  ptr(time.Date(2026, 6, 15, 0, 30, 0, 0, time.UTC)), Tag: "late"},
	}
	recs := domain.BuildDayRecords(sessions, now, target)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	if got := recs[0].Date.Format("2006-01-02"); got != "2026-06-15" {
		t.Errorf("local day grouping: got %s want 2026-06-15", got)
	}
	if recs[0].Total != time.Hour {
		t.Errorf("total: got %v want 1h", recs[0].Total)
	}
}
