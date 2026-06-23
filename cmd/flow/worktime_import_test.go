package main

import (
	"testing"
	"time"
)

func TestParseDateTimeBerlin(t *testing.T) {
	got, err := parseDateTimeBerlin("2026-05-04", "08:16")
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Europe/Berlin")
	want := time.Date(2026, 5, 4, 8, 16, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if _, err := parseDateTimeBerlin("nope", "08:16"); err == nil {
		t.Fatal("bad date should error")
	}
}

func TestParseLogLine(t *testing.T) {
	// valid line: 08:16→16:18 = 28920s
	e, ok, err := parseLogLine(5, "2026-05-04\t08:16\t16:18\t28920")
	if err != nil || !ok {
		t.Fatalf("valid line: ok=%v err=%v", ok, err)
	}
	if e.Seconds != 28920 || e.Stop.Sub(e.Start) != 8*time.Hour+2*time.Minute {
		t.Fatalf("entry = %+v", e)
	}
	// blank line → ok=false, no error
	if _, ok, err := parseLogLine(1, "   "); ok || err != nil {
		t.Fatalf("blank: ok=%v err=%v", ok, err)
	}
	// malformed: too few columns
	if _, _, err := parseLogLine(2, "2026-05-04\t08:16"); err == nil {
		t.Fatal("too few columns should error")
	}
	// malformed: bad time
	if _, _, err := parseLogLine(3, "2026-05-04\t8h16\t16:18\t10"); err == nil {
		t.Fatal("bad time should error")
	}
	// anomaly line still parses (seconds wildly off, clock times valid)
	e2, ok, err := parseLogLine(1, "2026-04-24\t07:34\t07:42\t259703")
	if err != nil || !ok || e2.Seconds != 259703 {
		t.Fatalf("anomaly: ok=%v err=%v e=%+v", ok, err, e2)
	}
}
