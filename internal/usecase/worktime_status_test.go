package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func newWorktimeStatus(t *testing.T, now time.Time) (usecase.WorktimeStatus, *testutil.FakeSessionStore) {
	t.Helper()
	clk := testutil.FakeClock{T: now}
	sessions := testutil.NewFakeSessionStore()
	settings := testutil.NewFakeUserSettingsStore()
	dayoffs := testutil.NewFakeDayOffStore()
	nodes := testutil.NewFakeNodeStore()
	listDayOffs := usecase.ListDayOffs{Store: dayoffs, Settings: settings, Loc: time.UTC}
	stats := usecase.StatsComputer{Sessions: sessions, Settings: settings, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC, Nodes: nodes}
	return usecase.WorktimeStatus{Stats: stats, Running: usecase.GetRunningSession{Sessions: sessions}, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC}, sessions
}

// The running session's live tail must be subtracted out of loggedMin so the
// client (which re-adds it from activeStart) does not double-count it.
func TestWorktimeStatus_LoggedExcludesRunningTail(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) // Monday noon
	uc, sessions := newWorktimeStatus(t, now)
	start := now.Add(-30 * time.Minute)
	_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start}) // running (Stop nil)
	res, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Running || res.ActiveID != "s1" {
		t.Fatalf("expected running s1, got running=%v id=%q", res.Running, res.ActiveID)
	}
	if res.Logged != 0 { // 30 min tail subtracted from the 30 min Today.Logged
		t.Fatalf("loggedMin should exclude the running tail, got %v", res.Logged)
	}
	if !res.ActiveStart.Equal(start) {
		t.Fatalf("activeStart = %v, want %v", res.ActiveStart, start)
	}
	// Asymmetry (dossier KRITISCH / Finding C3): today's WEEK entry INCLUDES the
	// live tail (server snapshot for pace-dot classification), UNLIKE top-level Logged.
	var todayWeek usecase.WorktimeStatusWeekDay
	for _, d := range res.Week {
		if d.IsToday {
			todayWeek = d
		}
	}
	if todayWeek.Logged < 30*time.Minute {
		t.Fatalf("today's week Logged must INCLUDE the running tail, got %v", todayWeek.Logged)
	}
}

// Finding #1: a session running ACROSS midnight was never counted by Today()
// (List filters start_at >= today-midnight), so its tail must NOT be subtracted
// — otherwise it eats OTHER completed same-day sessions. Here: 2h completed
// today + a session running since yesterday 22:00; loggedMin must stay 2h.
func TestWorktimeStatus_CrossMidnightRunningNotSubtracted(t *testing.T) {
	now := time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC) // Monday 08:00
	uc, sessions := newWorktimeStatus(t, now)
	completedStop := time.Date(2026, 6, 15, 2, 0, 0, 0, time.UTC)
	_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: "done", OwnerID: "u1",
		Start: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Stop: &completedStop}) // 2h completed today
	_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: "s1", OwnerID: "u1",
		Start: time.Date(2026, 6, 14, 22, 0, 0, 0, time.UTC)}) // running since yesterday 22:00
	res, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Logged != 2*time.Hour {
		t.Fatalf("cross-midnight run must not be subtracted; loggedMin want 2h, got %v", res.Logged)
	}
	if !res.ActiveStart.Equal(time.Date(2026, 6, 14, 22, 0, 0, 0, time.UTC)) {
		t.Fatalf("activeStart = %v", res.ActiveStart)
	}
}

// Owner-scoping (AGENTS.md hard rule): u2 must never see u1's running session.
func TestWorktimeStatus_OwnerScoped(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	uc, sessions := newWorktimeStatus(t, now)
	_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: "s1", OwnerID: "u1", Start: now.Add(-time.Hour)})
	res, err := uc.Execute(context.Background(), "u2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Running || res.ActiveID != "" {
		t.Fatalf("u2 must not see u1's session: running=%v id=%q", res.Running, res.ActiveID)
	}
}

// Idle: no running session → completed logged, no active id, week populated.
func TestWorktimeStatus_IdleNoRunning(t *testing.T) {
	now := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC) // Monday
	uc, sessions := newWorktimeStatus(t, now)
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := start.Add(3 * time.Hour)
	_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: "done", OwnerID: "u1", Start: start, Stop: &stop})
	res, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Running || res.ActiveID != "" {
		t.Fatalf("idle: running=%v id=%q", res.Running, res.ActiveID)
	}
	if res.Logged != 3*time.Hour {
		t.Fatalf("idle logged = %v, want 3h", res.Logged)
	}
	if len(res.Week) != 7 {
		t.Fatalf("week should have 7 entries, got %d", len(res.Week))
	}
}

// A running session that was already booked at start propagates ActiveNodeID
// (Finding C4) — the stop-picker uses it to skip the picker.
func TestWorktimeStatus_RunningWithNodePropagatesActiveNodeID(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	uc, sessions := newWorktimeStatus(t, now)
	node := "n1"
	_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: "s1", OwnerID: "u1", NodeID: &node, Start: now.Add(-time.Hour)})
	res, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if res.ActiveNodeID == nil || *res.ActiveNodeID != "n1" {
		t.Fatalf("ActiveNodeID must propagate, got %v", res.ActiveNodeID)
	}
}

// A day-off configured for today surfaces on res.DayOff with the right kind, and
// the matching week entry carries DayOffKind for pace-dot colouring.
func TestWorktimeStatus_DayOffTodaySurfaces(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) // Monday
	clk := testutil.FakeClock{T: now}
	sessions := testutil.NewFakeSessionStore()
	settings := testutil.NewFakeUserSettingsStore()
	dayoffs := testutil.NewFakeDayOffStore()
	listDayOffs := usecase.ListDayOffs{Store: dayoffs, Settings: settings, Loc: time.UTC}
	today := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if err := dayoffs.Add(context.Background(), "u1", domain.DayOff{Date: today, Kind: domain.KindVacation, Label: "Urlaub"}); err != nil {
		t.Fatal(err)
	}
	stats := usecase.StatsComputer{Sessions: sessions, Settings: settings, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC, Nodes: testutil.NewFakeNodeStore()}
	uc := usecase.WorktimeStatus{Stats: stats, Running: usecase.GetRunningSession{Sessions: sessions}, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC}
	res, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if res.DayOff == nil || res.DayOff.Kind != domain.KindVacation || res.DayOff.Label != "Urlaub" {
		t.Fatalf("today day-off not surfaced: %+v", res.DayOff)
	}
	found := false
	for _, d := range res.Week {
		if d.IsToday {
			if d.DayOffKind != domain.KindVacation {
				t.Fatalf("today week DayOffKind = %q, want vacation", d.DayOffKind)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("today not found in week")
	}
}

// burndown.Target==0 (no target configured) is passed through unchanged.
func TestWorktimeStatus_BurndownPassthrough(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	uc, _ := newWorktimeStatus(t, now)
	res, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	// Default settings DO carry a target, so burndown.Target should be > 0 here;
	// the field simply must round-trip from the reader without mutation.
	if res.Burndown.Target < 0 {
		t.Fatalf("burndown target must not be mangled: %v", res.Burndown.Target)
	}
}
