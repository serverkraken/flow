package tui

import (
	"strings"
	"testing"
	"time"
)

func TestExportPresetRange(t *testing.T) {
	// Montag 2026-06-15 als "now".
	now := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)
	cases := []struct {
		preset, from, to string
	}{
		{"monat", "2026-06-01", "2026-06-15"},
		{"kw", "2026-06-15", "2026-06-15"},      // Montag → from==today
		{"letzter", "2026-05-01", "2026-05-31"}, // ganzer Vormonat
	}
	for _, c := range cases {
		from, to := exportPresetRange(c.preset, now)
		if from != c.from || to != c.to {
			t.Errorf("%s: got %s..%s want %s..%s", c.preset, from, to, c.from, c.to)
		}
	}
}

func TestExportPresetRange_KWMidweek(t *testing.T) {
	// Mittwoch 2026-06-17 → KW-Start Montag 2026-06-15.
	now := time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)
	from, to := exportPresetRange("kw", now)
	if from != "2026-06-15" || to != "2026-06-17" {
		t.Errorf("kw midweek: got %s..%s want 2026-06-15..2026-06-17", from, to)
	}
}

func TestDefaultExportPath(t *testing.T) {
	got := defaultExportPath("2026-06-01", "2026-06-30", "md")
	if got != "~/Downloads/flow-export-2026-06-01_2026-06-30.md" {
		t.Errorf("got %q", got)
	}
	if g := defaultExportPath("2026-06-01", "2026-06-30", "csv"); !strings.HasSuffix(g, ".csv") {
		t.Errorf("csv ext: got %q", g)
	}
}

func TestExpandHome(t *testing.T) {
	if got := expandHome("/tmp/x.csv"); got != "/tmp/x.csv" {
		t.Errorf("absolute path must pass through: %q", got)
	}
	got := expandHome("~/Downloads/x.csv")
	if strings.HasPrefix(got, "~") || !strings.HasSuffix(got, "/Downloads/x.csv") {
		t.Errorf("~ not expanded: %q", got)
	}
}

func TestCycleFormatAndPreset(t *testing.T) {
	if cycleFormat("csv", +1) != "json" || cycleFormat("json", +1) != "md" || cycleFormat("md", +1) != "csv" {
		t.Error("cycleFormat forward wrong")
	}
	if cycleFormat("csv", -1) != "md" {
		t.Error("cycleFormat backward wrong")
	}
	if cyclePreset("kw", +1) != "monat" || cyclePreset("monat", +1) != "letzter" || cyclePreset("letzter", +1) != "custom" || cyclePreset("custom", +1) != "kw" {
		t.Error("cyclePreset forward wrong")
	}
	if cyclePreset("kw", -1) != "custom" {
		t.Error("cyclePreset backward wrong")
	}
}

func TestExportPresetRange_LetzterJanuaryBoundary(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	from, to := exportPresetRange("letzter", now)
	if from != "2025-12-01" || to != "2025-12-31" {
		t.Errorf("letzter in January: got %s..%s want 2025-12-01..2025-12-31", from, to)
	}
}
