package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

func TestExportOpenSetsDefaults(t *testing.T) {
	m := New(nil, "tester")
	m.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) // Montag
	next, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	mm := next.(Model)
	if !mm.showExport {
		t.Fatal("e should open the export overlay")
	}
	if mm.expFormat != "md" {
		t.Errorf("default format md, got %q", mm.expFormat)
	}
	if mm.expPreset != "monat" {
		t.Errorf("default preset monat, got %q", mm.expPreset)
	}
	if mm.expFrom != "2026-06-01" || mm.expTo != "2026-06-15" {
		t.Errorf("default range got %s..%s", mm.expFrom, mm.expTo)
	}
	if mm.expPath != "~/Downloads/flow-export-2026-06-01_2026-06-15.md" {
		t.Errorf("default path got %q", mm.expPath)
	}
}

func TestExportEscCloses(t *testing.T) {
	m := New(nil, "tester")
	next, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	m = next.(Model)
	next2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if next2.(Model).showExport {
		t.Fatal("esc should close the export overlay")
	}
}

func TestExportViewRenders(t *testing.T) {
	m := New(nil, "tester")
	m.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	next, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	out := next.(Model).View().Content
	for _, want := range []string{"Export", "Format", "2026-06-01", "md"} {
		if !strings.Contains(out, want) {
			t.Errorf("export view missing %q:\n%s", want, out)
		}
	}
}

func TestMainViewFooterHasExportHint(t *testing.T) {
	m := New(nil, "tester")
	if !strings.Contains(m.View().Content, "e export") {
		t.Errorf("main footer missing 'e export':\n%s", m.View().Content)
	}
}
