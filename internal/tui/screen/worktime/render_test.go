package worktime

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/tui/theme"
)

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plain(s string) string { return ansiRE.ReplaceAllString(s, "") }

func TestRenderBody_HeadlineBarSummarySessions(t *testing.T) {
	pal := theme.Load()
	loc := time.UTC
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, loc)
	c1Start := time.Date(2026, 6, 14, 9, 0, 0, 0, loc)
	c1Stop := time.Date(2026, 6, 14, 10, 0, 0, 0, loc)
	active := time.Date(2026, 6, 14, 11, 0, 0, 0, loc)
	st := todayState{
		Completed: []completedSession{{ID: "a", Start: c1Start, Stop: c1Stop, Elapsed: time.Hour, Tag: "deep"}},
		Running:   true, Active: &active, ActiveID: "run", Logged: time.Hour, Target: 8 * time.Hour,
	}
	body := plain(renderBody(st, 0, 80, 24, now, nil, pal))

	if !strings.Contains(body, "So · 14.06.2026") {
		t.Errorf("missing date line:\n%s", body)
	}
	if !strings.Contains(body, "läuft") {
		t.Errorf("missing running badge")
	}
	if !strings.Contains(body, "Ziel 8h 00m") || !strings.Contains(body, "ETA") {
		t.Errorf("missing summary/ETA:\n%s", body)
	}
	if !strings.Contains(body, "[deep]") {
		t.Errorf("missing tag hint")
	}
	if !strings.Contains(body, "09:00 → 10:00") {
		t.Errorf("missing completed session line")
	}
	// The running row ticks at second resolution so the clock visibly moves.
	if !strings.Contains(body, "11:00 → …") || !strings.Contains(body, "1h 00m 00s") {
		t.Errorf("running row should show live seconds:\n%s", body)
	}
}

func TestRenderBody_ShowsProjectBeforeTag(t *testing.T) {
	pal := theme.Load()
	loc := time.UTC
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, loc)
	c1Start := time.Date(2026, 6, 14, 9, 0, 0, 0, loc)
	c1Stop := time.Date(2026, 6, 14, 10, 0, 0, 0, loc)
	st := todayState{
		Completed: []completedSession{
			{ID: "a", Start: c1Start, Stop: c1Stop, Elapsed: time.Hour, Tag: "deep", Project: "Flow"},
			{ID: "b", Start: c1Start.Add(2 * time.Hour), Stop: c1Stop.Add(2 * time.Hour), Elapsed: time.Hour},
		},
		Target: 8 * time.Hour,
	}
	body := plain(renderBody(st, 0, 80, 24, now, nil, pal))
	// Project name sits before the tag in the same trailing hint.
	if !strings.Contains(body, "Flow [deep]") {
		t.Errorf("project should render before tag as %q:\n%s", "Flow [deep]", body)
	}
	// A session without a project still renders its row, just no project name.
	if !strings.Contains(body, "11:00 → 12:00") {
		t.Errorf("project-less session row missing:\n%s", body)
	}
}

func TestRenderBody_EmptyState(t *testing.T) {
	pal := theme.Load()
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	body := plain(renderBody(todayState{Target: 8 * time.Hour}, 0, 80, 24, now, nil, pal))
	if !strings.Contains(body, "Noch nichts erfasst") {
		t.Errorf("missing empty state:\n%s", body)
	}
}

func TestRenderBody_GapSeparator(t *testing.T) {
	pal := theme.Load()
	loc := time.UTC
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, loc)
	mk := func(h1, h2 int, gap time.Duration) completedSession {
		s := time.Date(2026, 6, 14, h1, 0, 0, 0, loc)
		e := time.Date(2026, 6, 14, h2, 0, 0, 0, loc)
		return completedSession{Start: s, Stop: e, Elapsed: e.Sub(s), GapBefore: gap}
	}
	st := todayState{Completed: []completedSession{mk(9, 10, 0), mk(11, 12, time.Hour)}, Target: 8 * time.Hour}
	body := plain(renderBody(st, 0, 80, 24, now, nil, pal))
	if !strings.Contains(body, "Pause 1h 00m") {
		t.Errorf("missing gap separator:\n%s", body)
	}
}

func TestTotalThresholdColor_NoTargetIsNotSuccess(t *testing.T) {
	pal := theme.Load()
	// no target, time logged, idle -> muted (NOT success/green)
	if got := totalThresholdColor(pal, 3*time.Hour, 0, false); got != pal.FgMuted {
		t.Errorf("no-target idle = %v, want FgMuted", got)
	}
	// no target, running -> active
	if got := totalThresholdColor(pal, 3*time.Hour, 0, true); got != pal.Sem().Active {
		t.Errorf("no-target running = %v, want Active", got)
	}
}

func TestTotalThresholdColor_Thresholds(t *testing.T) {
	pal := theme.Load()
	sem := pal.Sem()
	target := 8 * time.Hour
	cases := []struct {
		name    string
		total   time.Duration
		running bool
		want    interface{}
	}{
		{"way over -> danger", target + 5*time.Hour, false, sem.Danger},
		{"at target -> success", target, false, sem.Success},
		{"running near target -> warning", target - time.Hour, true, sem.Warning},
		{"running far -> active", target - 5*time.Hour, true, sem.Active},
		{"idle under -> muted", target - 5*time.Hour, false, pal.FgMuted},
	}
	for _, c := range cases {
		if got := totalThresholdColor(pal, c.total, target, c.running); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestTodayStatusBadge_AllStates(t *testing.T) {
	pal := theme.Load()
	sem := pal.Sem()
	cases := []struct {
		running, achieved bool
		wantLabel         string
		wantColor         interface{}
	}{
		{true, true, "läuft ✓", sem.Success},
		{true, false, "läuft", sem.Active},
		{false, true, "Ziel erreicht", sem.Success},
		{false, false, "pausiert", pal.FgMuted},
	}
	for _, c := range cases {
		_, label, col := todayStatusBadge(pal, c.running, c.achieved)
		if label != c.wantLabel || col != c.wantColor {
			t.Errorf("badge(%v,%v) = (%q,%v), want (%q,%v)", c.running, c.achieved, label, col, c.wantLabel, c.wantColor)
		}
	}
}
