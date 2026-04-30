package worktime_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

func TestWriteMarkdownBrief_Week_HasOverviewAndDays(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	worktime.ResetCachesForTesting()

	// Seed two sessions on a Monday in the same week.
	mon := time.Date(2026, 4, 27, 9, 0, 0, 0, time.Local)
	if err := worktime.AddManual(
		mon, mon, mon.Add(2*time.Hour),
	); err != nil {
		t.Fatal(err)
	}
	if err := worktime.AddManual(
		mon, mon.Add(3*time.Hour), mon.Add(7*time.Hour),
	); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := worktime.WriteMarkdownBrief(&buf, mon, worktime.ReportWeek); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	mustContain := []string{
		"# Worktime · KW 18",   // 2026-04-27 is in ISO week 18
		"## Übersicht",
		"## Tage",
		"**Mo, 27.04.**",
		"09:00–11:00",
		"12:00–16:00",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("brief missing %q in:\n%s", s, out)
		}
	}
}

func TestWriteMarkdownBrief_Month_TitleHasMonth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	worktime.ResetCachesForTesting()

	ref := time.Date(2026, 4, 15, 0, 0, 0, 0, time.Local)
	var buf bytes.Buffer
	if err := worktime.WriteMarkdownBrief(&buf, ref, worktime.ReportMonth); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Apr 2026") {
		t.Errorf("month brief should mention Apr 2026, got:\n%s", buf.String())
	}
}
