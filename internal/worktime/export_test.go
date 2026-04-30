package worktime_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

func TestParseRange_Empty(t *testing.T) {
	t.Parallel()
	r, err := worktime.ParseRange(time.Now(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !r.From.IsZero() || !r.To.IsZero() {
		t.Errorf("empty range = %+v, want zero", r)
	}
}

func TestParseRange_Today(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	r, err := worktime.ParseRange(now, "today")
	if err != nil {
		t.Fatal(err)
	}
	if r.From.Day() != 29 || r.To.Day() != 30 {
		t.Errorf("today range = %+v", r)
	}
}

func TestParseRange_Week(t *testing.T) {
	t.Parallel()
	// 2026-04-29 is a Wednesday. ISO week starts Monday.
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	r, err := worktime.ParseRange(now, "week")
	if err != nil {
		t.Fatal(err)
	}
	if r.From.Weekday() != time.Monday {
		t.Errorf("week start = %v, want Monday", r.From.Weekday())
	}
	if r.To.Sub(r.From) != 7*24*time.Hour {
		t.Errorf("week duration = %v, want 7d", r.To.Sub(r.From))
	}
}

func TestParseRange_Year(t *testing.T) {
	t.Parallel()
	r, err := worktime.ParseRange(time.Now(), "2026")
	if err != nil {
		t.Fatal(err)
	}
	if r.From.Year() != 2026 || r.To.Year() != 2027 {
		t.Errorf("year range = %+v", r)
	}
}

func TestParseRange_Month(t *testing.T) {
	t.Parallel()
	r, err := worktime.ParseRange(time.Now(), "2026-04")
	if err != nil {
		t.Fatal(err)
	}
	if r.From.Month() != time.April || r.To.Month() != time.May {
		t.Errorf("month range = %+v", r)
	}
}

func TestParseRange_DateRange(t *testing.T) {
	t.Parallel()
	r, err := worktime.ParseRange(time.Now(), "2026-04-01..2026-04-15")
	if err != nil {
		t.Fatal(err)
	}
	if r.From.Day() != 1 || r.To.Day() != 16 { // To is exclusive
		t.Errorf("date range = %+v", r)
	}
}

func TestParseRange_Bad(t *testing.T) {
	t.Parallel()
	if _, err := worktime.ParseRange(time.Now(), "yesterday"); err == nil {
		t.Error("expected error for unsupported expr")
	}
}

func TestExportCSV_HasHeader(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	writeTmuxFiles(t, dir, "", "2026-04-28\t09:00\t17:00\t28800\tdeep\tnotes\n")

	var buf bytes.Buffer
	if err := worktime.ExportCSV(&buf, worktime.Range{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "date,start,stop,elapsed_seconds,tag,note") {
		t.Errorf("missing CSV header in: %q", out)
	}
	if !strings.Contains(out, "deep") || !strings.Contains(out, "notes") {
		t.Errorf("CSV missing tag/note: %q", out)
	}
}

func TestExportJSON_StructureAndContent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	writeTmuxFiles(t, dir, "", "2026-04-28\t09:00\t17:00\t28800\tdeep\tnotes\n")

	var buf bytes.Buffer
	if err := worktime.ExportJSON(&buf, worktime.Range{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`"date": "2026-04-28"`,
		`"elapsed_seconds": 28800`,
		`"tag": "deep"`,
		`"note": "notes"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON missing %q in: %s", want, out)
		}
	}
}
