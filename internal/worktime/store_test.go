package worktime_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

func writeTmuxFiles(t *testing.T, dir, stateEpoch, logContent string) {
	t.Helper()
	tmuxDir := filepath.Join(dir, ".tmux")
	if err := os.MkdirAll(tmuxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if stateEpoch != "" {
		if err := os.WriteFile(filepath.Join(tmuxDir, "worktime.state"), []byte(stateEpoch), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if logContent != "" {
		if err := os.WriteFile(filepath.Join(tmuxDir, "worktime.log"), []byte(logContent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadToday_NoFiles_ReturnsEmptyDay(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	day, err := worktime.LoadToday()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if day.IsRunning() {
		t.Error("expected no active session")
	}
	if len(day.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(day.Sessions))
	}
	if day.Target != worktime.TargetHours {
		t.Errorf("Target = %v, want %v", day.Target, worktime.TargetHours)
	}
}

func TestLoadToday_ActiveSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	start := time.Now().Add(-90 * time.Minute)
	writeTmuxFiles(t, dir, fmt.Sprintf("%d", start.Unix()), "")

	day, err := worktime.LoadToday()
	if err != nil {
		t.Fatal(err)
	}
	if !day.IsRunning() {
		t.Error("expected active session")
	}
}

func TestLoadToday_TodaySessionsFiltered(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	today := time.Now().Format("2006-01-02")
	log := today + "\t09:00\t12:00\t10800\n" +
		"2020-01-01\t10:00\t11:00\t3600\n"

	writeTmuxFiles(t, dir, "", log)

	day, err := worktime.LoadToday()
	if err != nil {
		t.Fatal(err)
	}
	if len(day.Sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(day.Sessions))
	}
	if day.Logged != 3*time.Hour {
		t.Errorf("Logged = %v, want 3h", day.Logged)
	}
}

func TestDay_Total_NoActiveSession(t *testing.T) {
	t.Parallel()
	d := worktime.Day{
		Logged: 2 * time.Hour,
		Target: worktime.TargetHours,
	}
	if got := d.Total(time.Now()); got != 2*time.Hour {
		t.Errorf("Total() = %v, want 2h", got)
	}
}

func TestDay_Total_WithActiveSession(t *testing.T) {
	t.Parallel()
	now := time.Now()
	start := now.Add(-30 * time.Minute)
	d := worktime.Day{
		Logged: time.Hour,
		Active: &start,
		Target: worktime.TargetHours,
	}
	got := d.Total(now)
	want := 90 * time.Minute
	if got < want-time.Second || got > want+time.Second {
		t.Errorf("Total() = %v, want ~90min", got)
	}
}

func TestParseStartArg_Now(t *testing.T) {
	t.Parallel()
	before := time.Now()
	ts, err := worktime.ParseStartArg("")
	after := time.Now()
	if err != nil {
		t.Fatal(err)
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("ParseStartArg('') = %v, want time.Now()", ts)
	}
}

func TestParseStartArg_MinusMinutes(t *testing.T) {
	t.Parallel()
	ts, err := worktime.ParseStartArg("-30m")
	if err != nil {
		t.Fatal(err)
	}
	diff := time.Since(ts)
	if diff < 29*time.Minute || diff > 31*time.Minute {
		t.Errorf("ParseStartArg('-30m') offset = %v, want ~30m", diff)
	}
}

func TestParseStartArg_HoursMinutes(t *testing.T) {
	t.Parallel()
	ts, err := worktime.ParseStartArg("-1h30m")
	if err != nil {
		t.Fatal(err)
	}
	diff := time.Since(ts)
	if diff < 89*time.Minute || diff > 91*time.Minute {
		t.Errorf("ParseStartArg('-1h30m') offset = %v, want ~90m", diff)
	}
}

func TestStatusSegment_EmptyState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if got := worktime.StatusSegment(); got != "" {
		t.Errorf("StatusSegment empty state = %q, want empty", got)
	}
}

func TestStatusSegment_PausedShowsTotal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	today := time.Now().Format("2006-01-02")
	log := today + "\t09:00\t12:00\t10800\n"
	writeTmuxFiles(t, dir, "", log)

	got := worktime.StatusSegment()
	if got == "" {
		t.Fatal("expected non-empty status segment")
	}
	if !strings.Contains(got, "⏸") {
		t.Errorf("paused segment should contain ⏸, got %q", got)
	}
	if !strings.Contains(got, "03:00") {
		t.Errorf("segment should show total 03:00, got %q", got)
	}
}

func TestStatusSegment_RunningIncludesElapsed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	start := time.Now().Add(-45 * time.Minute)
	writeTmuxFiles(t, dir, fmt.Sprintf("%d", start.Unix()), "")

	got := worktime.StatusSegment()
	if !strings.Contains(got, "⏱") {
		t.Errorf("running segment should contain ⏱, got %q", got)
	}
	if !strings.Contains(got, "▶") {
		t.Errorf("running segment should contain ▶ active marker, got %q", got)
	}
}

func TestStatusSegment_AchievedShowsCheckmark(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Log 9 hours today, target is 8h.
	today := time.Now().Format("2006-01-02")
	log := today + "\t08:00\t17:00\t32400\n"
	writeTmuxFiles(t, dir, "", log)

	got := worktime.StatusSegment()
	if !strings.Contains(got, "✓") {
		t.Errorf("achieved segment should contain ✓, got %q", got)
	}
}
