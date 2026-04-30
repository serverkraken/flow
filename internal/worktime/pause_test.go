package worktime_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

func TestStart_ErrAlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := worktime.Start(time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	err := worktime.Start(time.Now())
	if !errors.Is(err, worktime.ErrAlreadyRunning) {
		t.Errorf("expected ErrAlreadyRunning, got %v", err)
	}
}

func TestStartForce_OverwritesRunning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := worktime.Start(time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := worktime.StartForce(time.Now()); err != nil {
		t.Fatalf("StartForce should overwrite, got %v", err)
	}
	day, err := worktime.LoadToday()
	if err != nil {
		t.Fatal(err)
	}
	if !day.IsRunning() {
		t.Fatal("expected session to be running after StartForce")
	}
	// The start time should now be ~now, not -1h.
	if d := time.Since(*day.Active); d > 5*time.Second {
		t.Errorf("expected fresh start, but Active is %v old", d)
	}
}

func TestPauseResume_Cycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := worktime.Start(time.Now().Add(-30 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := worktime.Pause(); err != nil {
		t.Fatal(err)
	}
	day, err := worktime.LoadToday()
	if err != nil {
		t.Fatal(err)
	}
	if day.IsRunning() {
		t.Error("after Pause, no session should be running")
	}
	if !day.IsPaused() {
		t.Error("after Pause, IsPaused should be true")
	}
	if day.PausedAt == nil {
		t.Fatal("PausedAt should be set after Pause")
	}

	if err := worktime.Resume(); err != nil {
		t.Fatal(err)
	}
	day, err = worktime.LoadToday()
	if err != nil {
		t.Fatal(err)
	}
	if !day.IsRunning() {
		t.Error("after Resume, session should be running")
	}
	if day.IsPaused() {
		t.Error("after Resume, IsPaused should be false")
	}
}

func TestPause_WhenIdle_NoOp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if _, err := worktime.Pause(); err != nil {
		t.Errorf("Pause when idle should be silent no-op, got %v", err)
	}
}

func TestStop_ClearsPauseMarker(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := worktime.Start(time.Now().Add(-30 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := worktime.Pause(); err != nil {
		t.Fatal(err)
	}
	// User starts a fresh session and stops normally — pause marker should clear.
	if err := worktime.Resume(); err != nil {
		t.Fatal(err)
	}
	if _, err := worktime.Stop(); err != nil {
		t.Fatal(err)
	}
	day, err := worktime.LoadToday()
	if err != nil {
		t.Fatal(err)
	}
	if day.IsPaused() {
		t.Error("Stop should clear the pause marker")
	}
}
