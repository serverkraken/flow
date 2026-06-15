package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

// --- helpers ----------------------------------------------------------------

func newKeyPress(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: text}
}

func newKeyEsc() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEsc}
}

// --- bar --------------------------------------------------------------------

func TestBar(t *testing.T) {
	cases := []struct {
		logged, target, width int
		want                  string
	}{
		{60, 120, 10, "[█████░░░░░]"},  // half
		{0, 480, 10, "[░░░░░░░░░░]"},   // all empty
		{600, 480, 10, "[██████████]"}, // all full (clamped)
	}
	for _, tc := range cases {
		got := bar(tc.logged, tc.target, tc.width)
		if got != tc.want {
			t.Errorf("bar(%d, %d, %d) = %q, want %q", tc.logged, tc.target, tc.width, got, tc.want)
		}
	}
}

// --- weekView ---------------------------------------------------------------

func TestWeekView_RendersDays(t *testing.T) {
	m := New(nil, "tester")
	m.week = []apiclient.WeekDay{
		{Date: "2026-06-15", LoggedMin: 60, TargetMin: 480, IsToday: true, Workday: true},
	}
	m.showWeek = true

	out := m.View().Content
	if !strings.Contains(out, "2026-06-15") {
		t.Fatalf("weekView missing date '2026-06-15':\n%s", out)
	}
	if !strings.Contains(out, "Woche") {
		t.Fatalf("weekView missing 'Woche':\n%s", out)
	}
}

// --- statsView --------------------------------------------------------------

func TestStatsView_RendersTotal(t *testing.T) {
	m := New(nil, "tester")
	m.showStats = true
	m.statsRng = "week"

	next, _ := m.Update(rangeLoadedMsg{rng: "week", stats: apiclient.Stats{TotalMin: 600}})
	m = next.(Model)

	out := m.View().Content
	if !strings.Contains(out, "Total") {
		t.Fatalf("statsView missing 'Total':\n%s", out)
	}
	if !strings.Contains(out, "Stats") {
		t.Fatalf("statsView missing 'Stats':\n%s", out)
	}
}

// --- key routing ------------------------------------------------------------

func TestWeekKey_SetsShowWeek(t *testing.T) {
	m := New(nil, "tester")
	next, _ := m.Update(newKeyPress("w"))
	m = next.(Model)
	if !m.showWeek {
		t.Fatal("w key should set showWeek=true")
	}
}

func TestTKey_SetsShowStats(t *testing.T) {
	m := New(nil, "tester")
	next, _ := m.Update(newKeyPress("t"))
	m = next.(Model)
	if !m.showStats {
		t.Fatal("t key should set showStats=true")
	}
	if m.statsRng != "week" {
		t.Fatalf("statsRng should default to 'week', got %q", m.statsRng)
	}
}

func TestEsc_ClosesWeekView(t *testing.T) {
	m := New(nil, "tester")
	m.showWeek = true
	next, _ := m.Update(newKeyEsc())
	m = next.(Model)
	if m.showWeek {
		t.Fatal("esc should close week view")
	}
}

func TestEsc_ClosesStatsView(t *testing.T) {
	m := New(nil, "tester")
	m.showStats = true
	next, _ := m.Update(newKeyEsc())
	m = next.(Model)
	if m.showStats {
		t.Fatal("esc should close stats view")
	}
}

func TestStatsRng_MSwitch(t *testing.T) {
	m := New(nil, "tester")
	m.showStats = true
	m.statsRng = "week"
	next, _ := m.Update(newKeyPress("m"))
	m = next.(Model)
	if m.statsRng != "month" {
		t.Fatalf("m key in stats view should set statsRng='month', got %q", m.statsRng)
	}
}

func TestStatsRng_WSwitch(t *testing.T) {
	m := New(nil, "tester")
	m.showStats = true
	m.statsRng = "month"
	next, _ := m.Update(newKeyPress("W"))
	m = next.(Model)
	if m.statsRng != "week" {
		t.Fatalf("W key in stats view should set statsRng='week', got %q", m.statsRng)
	}
}

// --- settings + target editor -----------------------------------------------

func TestSettingsLoadedStored(t *testing.T) {
	m := New(nil, "tester")
	next, _ := m.Update(settingsLoadedMsg{settings: apiclient.Settings{DefaultTargetMin: 480}})
	m = next.(Model)
	if m.settings.DefaultTargetMin != 480 {
		t.Fatalf("settings not stored, DefaultTargetMin=%d", m.settings.DefaultTargetMin)
	}
}

func TestDayOffView_RendersTargetConfig(t *testing.T) {
	m := New(nil, "tester")
	m.showDayOffs = true
	m.settings = apiclient.Settings{DefaultTargetMin: 480}

	out := m.View().Content
	if !strings.Contains(out, "Tagesziel") {
		t.Fatalf("dayOffView missing 'Tagesziel':\n%s", out)
	}
	if !strings.Contains(out, "8h 00m") {
		t.Fatalf("dayOffView missing '8h 00m' for 480 min target:\n%s", out)
	}
}

func TestDayOffView_WeekdayOverrides(t *testing.T) {
	m := New(nil, "tester")
	m.showDayOffs = true
	m.settings = apiclient.Settings{
		DefaultTargetMin: 480,
		WeekdayTargetMin: map[string]int{"5": 360},
	}

	out := m.View().Content
	if !strings.Contains(out, "Fr") {
		t.Fatalf("dayOffView missing weekday override 'Fr':\n%s", out)
	}
	if !strings.Contains(out, "6h 00m") {
		t.Fatalf("dayOffView missing override '6h 00m':\n%s", out)
	}
}

func TestTargetEdit_GKeyOpens(t *testing.T) {
	m := New(nil, "tester")
	m.showDayOffs = true
	next, _ := m.Update(newKeyPress("g"))
	m = next.(Model)
	if !m.editingTarget {
		t.Fatal("g key should open target edit mode")
	}
}

func TestTargetEdit_EscCancels(t *testing.T) {
	m := New(nil, "tester")
	m.showDayOffs = true
	m.editingTarget = true
	m.targetInput = "480"
	// When editingTarget=true, esc is routed through handleDayOffKey which
	// cancels the edit but keeps the dayoffs view open.
	next, _ := m.Update(newKeyEsc())
	m = next.(Model)
	if m.editingTarget {
		t.Fatal("esc should cancel target edit (editingTarget must be false)")
	}
	if !m.showDayOffs {
		t.Fatal("showDayOffs should remain true after esc from target edit")
	}
}

func TestParseMinutes(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"480", 480},
		{"0", 0},
		{"", 0},
		{"abc", 0},
		{"360", 360},
	}
	for _, tc := range cases {
		got := parseMinutes(tc.in)
		if got != tc.want {
			t.Errorf("parseMinutes(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestWeekdayShort(t *testing.T) {
	cases := map[string]string{
		"0": "So", "1": "Mo", "2": "Di",
		"3": "Mi", "4": "Do", "5": "Fr", "6": "Sa",
	}
	for k, want := range cases {
		if got := weekdayShort(k); got != want {
			t.Errorf("weekdayShort(%q) = %q, want %q", k, got, want)
		}
	}
}
