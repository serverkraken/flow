package domain

import (
	"testing"
	"time"
)

func TestWorkSessionRunningWhenStopNil(t *testing.T) {
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	s, err := NewWorkSession("s1", "u1", nil, start)
	if err != nil {
		t.Fatalf("NewWorkSession: %v", err)
	}
	if !s.Running() {
		t.Fatal("a session with nil Stop must be running")
	}
	now := start.Add(90 * time.Minute)
	if got := s.Elapsed(now); got != 90*time.Minute {
		t.Fatalf("running elapsed = %v, want 90m", got)
	}
}

func TestWorkSessionElapsedWhenStopped(t *testing.T) {
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(30 * time.Minute)
	s, _ := NewWorkSession("s1", "u1", nil, start)
	s.Stop = &stop
	if s.Running() {
		t.Fatal("stopped session must not be running")
	}
	if got := s.Elapsed(time.Now()); got != 30*time.Minute {
		t.Fatalf("stopped elapsed = %v, want 30m", got)
	}
}

func TestNewWorkSessionValidationErrors(t *testing.T) {
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	if _, err := NewWorkSession("", "u1", nil, start); err == nil {
		t.Fatal("expected error for empty id")
	}
	if _, err := NewWorkSession("s1", "", nil, start); err == nil {
		t.Fatal("expected error for empty ownerID")
	}
}
