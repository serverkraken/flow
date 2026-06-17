package worktime

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func ptr[T any](v T) *T { return &v }

func TestReconstruct_FiltersTodayAndComputesFields(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, loc)
	mk := func(start, stop string) domain.WorkSession {
		s, _ := time.ParseInLocation("2006-01-02 15:04", start, loc)
		ws := domain.WorkSession{ID: start, Start: s}
		if stop != "" {
			e, _ := time.ParseInLocation("2006-01-02 15:04", stop, loc)
			ws.Stop = &e
		}
		return ws
	}
	sessions := []domain.WorkSession{
		mk("2026-06-13 09:00", "2026-06-13 17:00"), // yesterday — excluded
		mk("2026-06-14 09:00", "2026-06-14 10:00"), // today, 1h
		mk("2026-06-14 10:30", "2026-06-14 11:00"), // today, 0.5h (gap 30m before)
		mk("2026-06-14 11:30", ""),                 // today, running
	}
	today := apiclient.Today{TargetMin: 480, LoggedMin: 90, Running: true}

	st := reconstruct(today, sessions, now)

	if len(st.Completed) != 2 {
		t.Fatalf("Completed = %d, want 2 (today, stopped)", len(st.Completed))
	}
	if !st.Running || st.Active == nil || !st.Active.Equal(mk("2026-06-14 11:30", "").Start) {
		t.Fatalf("running/active wrong: %+v", st)
	}
	if st.Target != 8*time.Hour {
		t.Fatalf("Target = %v", st.Target)
	}
	if st.Logged != 90*time.Minute {
		t.Fatalf("Logged = %v, want 90m", st.Logged)
	}
	if got := st.Total(now); got != 2*time.Hour {
		t.Fatalf("Total = %v, want 2h", got)
	}
	if st.Completed[1].GapBefore != 30*time.Minute {
		t.Fatalf("gap = %v, want 30m", st.Completed[1].GapBefore)
	}
}

func TestReconstruct_ETA(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, loc)
	active := time.Date(2026, 6, 14, 11, 0, 0, 0, loc)
	st := todayState{Running: true, Active: &active, Logged: 6 * time.Hour, Target: 8 * time.Hour}
	eta, ok := st.ETA()
	if !ok || eta.Format("15:04") != "13:00" {
		t.Fatalf("ETA = %v ok=%v", eta, ok)
	}
	st.Target = 0
	if _, ok := st.ETA(); ok {
		t.Fatal("no-target ETA should be absent")
	}
	_ = now
}

func TestReconstruct_LocalTZBoundary(t *testing.T) {
	loc := time.FixedZone("CEST", 2*3600)
	now := time.Date(2026, 6, 14, 23, 45, 0, 0, loc)
	s := time.Date(2026, 6, 14, 23, 30, 0, 0, loc)
	e := time.Date(2026, 6, 14, 23, 40, 0, 0, loc)
	st := reconstruct(apiclient.Today{}, []domain.WorkSession{{ID: "x", Start: s, Stop: &e}}, now)
	if len(st.Completed) != 1 {
		t.Fatalf("late-night session dropped: %d", len(st.Completed))
	}
}
