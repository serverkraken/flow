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
