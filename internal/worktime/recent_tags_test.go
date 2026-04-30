package worktime_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

func TestRecentTags_Empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	tags, err := worktime.RecentTags(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 0 {
		t.Errorf("got %d tags, want 0", len(tags))
	}
}

func TestRecentTags_DistinctNewestFirst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	day := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	mk := func(start, stop string) {
		s, _ := worktime.ParseHM(start)
		e, _ := worktime.ParseHM(stop)
		base := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local)
		if err := worktime.AddManual(day, base.Add(s), base.Add(e)); err != nil {
			t.Fatal(err)
		}
	}
	mk("09:00", "10:00")
	mk("11:00", "12:00")
	mk("13:00", "14:00")
	mk("15:00", "16:00")

	// Tag them in increasing time order. RecentTags should return newest first.
	for i, tag := range []string{"meeting", "deep", "deep", "support"} {
		if err := worktime.SetTag(day, i, tag); err != nil {
			t.Fatal(err)
		}
	}

	tags, err := worktime.RecentTags(10)
	if err != nil {
		t.Fatal(err)
	}
	// Newest session is the 15:00–16:00 entry → "support" first.
	if len(tags) != 3 {
		t.Fatalf("got %d distinct tags, want 3 (support, deep, meeting): %v", len(tags), tags)
	}
	if tags[0] != "support" {
		t.Errorf("tags[0] = %q, want support", tags[0])
	}
}

func TestSessionsOverlap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	day := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	base := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	startD, _ := worktime.ParseHM("09:00")
	stopD, _ := worktime.ParseHM("12:00")
	if err := worktime.AddManual(day, base.Add(startD), base.Add(stopD)); err != nil {
		t.Fatal(err)
	}

	// Overlapping span: 11:00–13:00 hits the existing 09:00–12:00.
	overlapStart, _ := worktime.ParseHM("11:00")
	overlapStop, _ := worktime.ParseHM("13:00")
	hit, conflict, err := worktime.SessionsOverlap(day, base.Add(overlapStart), base.Add(overlapStop), -1)
	if err != nil {
		t.Fatal(err)
	}
	if !hit || conflict == nil {
		t.Error("expected overlap detection")
	}

	// Non-overlapping: 13:00–14:00.
	noStart, _ := worktime.ParseHM("13:00")
	noStop, _ := worktime.ParseHM("14:00")
	hit, _, err = worktime.SessionsOverlap(day, base.Add(noStart), base.Add(noStop), -1)
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Error("expected no overlap for adjacent span")
	}
}

func TestAddManual_RejectsOverlap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	day := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	base := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	a, _ := worktime.ParseHM("09:00")
	b, _ := worktime.ParseHM("12:00")
	if err := worktime.AddManual(day, base.Add(a), base.Add(b)); err != nil {
		t.Fatal(err)
	}
	// Try to add an overlap.
	c, _ := worktime.ParseHM("11:00")
	d, _ := worktime.ParseHM("13:00")
	err := worktime.AddManual(day, base.Add(c), base.Add(d))
	if err == nil {
		t.Fatal("expected overlap error")
	}
	if !errors.Is(err, worktime.ErrOverlap) {
		t.Errorf("expected ErrOverlap, got %v", err)
	}
}

func TestEditSession_OverlapWithItselfAllowed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	day := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	base := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	a, _ := worktime.ParseHM("09:00")
	b, _ := worktime.ParseHM("12:00")
	if err := worktime.AddManual(day, base.Add(a), base.Add(b)); err != nil {
		t.Fatal(err)
	}
	// Edit the same row to a fully-contained subset — must succeed.
	na, _ := worktime.ParseHM("09:30")
	nb, _ := worktime.ParseHM("11:30")
	if err := worktime.EditSession(day, 0, base.Add(na), base.Add(nb)); err != nil {
		t.Fatalf("self-edit should succeed: %v", err)
	}
}
