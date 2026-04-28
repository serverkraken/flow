package worktime_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/screen/worktime"
)

func writeTmuxFiles(t *testing.T, dir string, stateEpoch string, logContent string) {
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
	if day.Target != worktime.TargetHours*time.Hour {
		t.Errorf("Target = %v, want %v", day.Target, worktime.TargetHours*time.Hour)
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
	log := today + "\t09:00\t12:00\t10800\n" + // 3h today
		"2020-01-01\t10:00\t11:00\t3600\n" // old date, must be filtered

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
		Target: worktime.TargetHours * time.Hour,
	}
	got := d.Total(time.Now())
	if got != 2*time.Hour {
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
		Target: worktime.TargetHours * time.Hour,
	}
	got := d.Total(now)
	want := 90 * time.Minute
	if got < want-time.Second || got > want+time.Second {
		t.Errorf("Total() = %v, want ~90min", got)
	}
}
