package httpserver

import (
	"testing"
	"time"
)

// fixedClock satisfies ports.Clock for tests in this (internal) package.
type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestParseDayParam_DefaultsToToday(t *testing.T) {
	s := &Server{Clock: fixedClock{now: time.Date(2026, 6, 21, 14, 0, 0, 0, time.Local)}}
	got := parseDayParam(s, "")
	want := time.Date(2026, 6, 21, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("empty: got %v want %v", got, want)
	}
	got = parseDayParam(s, "2026-06-18")
	want = time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("valid: got %v want %v", got, want)
	}
	if !parseDayParam(s, "garbage").Equal(time.Date(2026, 6, 21, 0, 0, 0, 0, time.Local)) {
		t.Fatal("garbage should fall back to today")
	}
}

func TestDayTime_LocalHHMM(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	got, err := dayTime(day, "09:30")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 6, 18, 9, 30, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if _, err := dayTime(day, "nope"); err == nil {
		t.Fatal("expected error for bad HH:MM")
	}
}
