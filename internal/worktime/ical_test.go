package worktime_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

func TestExportICS_BasicShape(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	d1 := time.Date(2026, 4, 30, 0, 0, 0, 0, time.Local)
	d2 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	if err := worktime.AddDayOff(d1, worktime.KindHoliday, "Tag der Arbeit"); err != nil {
		t.Fatal(err)
	}
	if err := worktime.AddDayOff(d2, worktime.KindVacation, "Brückentag"); err != nil {
		t.Fatal(err)
	}

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.Local)
	var b strings.Builder
	if err := worktime.ExportICS(&b, from, to); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	for _, want := range []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//serverkraken//flow worktime//DE",
		"BEGIN:VEVENT",
		"DTSTART;VALUE=DATE:20260430",
		"DTEND;VALUE=DATE:20260501", // exclusive
		"DTSTART;VALUE=DATE:20260501",
		"DTEND;VALUE=DATE:20260502",
		"SUMMARY:Tag der Arbeit (Feiertag)",
		"SUMMARY:Brückentag (Urlaub)",
		"TRANSP:TRANSPARENT",
		"END:VEVENT",
		"END:VCALENDAR",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ics output missing %q\nfull output:\n%s", want, out)
		}
	}
	// CRLF line endings per RFC 5545.
	if !strings.Contains(out, "\r\n") {
		t.Error("expected CRLF line endings, got LF-only")
	}
}

func TestExportICS_EscapesSpecialChars(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	d := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
	// Label with comma + semicolon — both must be escaped.
	if err := worktime.AddDayOff(d, worktime.KindVacation, "Urlaub, Ausland; Spanien"); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.Local)
	var b strings.Builder
	if err := worktime.ExportICS(&b, from, to); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `Urlaub\, Ausland\; Spanien`) {
		t.Errorf("expected escaped label in output, got:\n%s", out)
	}
}

func TestExportICS_EmptyDoesNotCrash(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	from := time.Date(2030, 1, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2030, 12, 31, 0, 0, 0, 0, time.Local)
	var b strings.Builder
	if err := worktime.ExportICS(&b, from, to); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "BEGIN:VCALENDAR") {
		t.Error("empty export should still emit valid VCALENDAR envelope")
	}
}
