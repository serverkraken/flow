package datepicker_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/datepicker"
)

func TestDatepicker_valueRoundTrip(t *testing.T) {
	m := datepicker.New(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), theme.Default)
	if m.Value() != "2026-07-20" {
		t.Fatalf("Value = %q, want 2026-07-20", m.Value())
	}
	if err := m.SetValue("2024-02-29"); err != nil {
		t.Fatalf("SetValue err: %v", err)
	}
	if m.Value() != "2024-02-29" {
		t.Fatalf("after SetValue, Value = %q", m.Value())
	}
	if err := m.SetValue("not-a-date"); err == nil {
		t.Fatal("SetValue should reject bad input")
	}
}

func TestDatepicker_viewShowsSegmentsAndWeekday(t *testing.T) {
	m := datepicker.New(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), theme.Default)
	v := m.View()
	if !strings.Contains(v, "2026") || !strings.Contains(v, "07") || !strings.Contains(v, "20") {
		t.Fatalf("view missing date segments: %q", v)
	}
	if !strings.Contains(v, "Mo") { // 2026-07-20 is a Monday
		t.Fatalf("view should show weekday Mo: %q", v)
	}
}

func key(code rune) tea.KeyPressMsg  { return tea.KeyPressMsg{Code: code} }
func digit(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Text: s} }

func TestDatepicker_arrowStepsAndRollsOver(t *testing.T) {
	m := datepicker.New(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), theme.Default)
	// active segment defaults to year (0). Move to month (seg 1) and roll 12 -> 1.
	m = m.Update(key(tea.KeyRight)) // seg=month
	m = m.Update(key(tea.KeyUp))    // 12 -> 1
	if m.Value() != "2026-01-31" {
		t.Fatalf("month rollover: %q, want 2026-01-31", m.Value())
	}
}

func TestDatepicker_dayClampsOnMonthChange(t *testing.T) {
	m := datepicker.New(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), theme.Default)
	m = m.Update(key(tea.KeyRight)) // seg=month
	m = m.Update(key(tea.KeyUp))    // Jan -> Feb; day 31 must clamp to 28 (2026 not leap)
	if m.Value() != "2026-02-28" {
		t.Fatalf("day clamp: %q, want 2026-02-28", m.Value())
	}
}

func TestDatepicker_dayRollsWithinMonth(t *testing.T) {
	m := datepicker.New(time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), theme.Default)
	m = m.Update(key(tea.KeyRight)) // seg=month
	m = m.Update(key(tea.KeyRight)) // seg=day
	m = m.Update(key(tea.KeyUp))    // 28 -> rolls to 1 (Feb has 28)
	if m.Value() != "2026-02-01" {
		t.Fatalf("day rollover: %q, want 2026-02-01", m.Value())
	}
}

func TestDatepicker_digitEntryFillsSegments(t *testing.T) {
	m := datepicker.New(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), theme.Default)
	// year: 2026
	for _, c := range []string{"2", "0", "2", "6"} {
		m = m.Update(digit(c))
	}
	// auto-advanced to month: "07"
	m = m.Update(digit("0"))
	m = m.Update(digit("7"))
	// auto-advanced to day: "2","0"
	m = m.Update(digit("2"))
	m = m.Update(digit("0"))
	if m.Value() != "2026-07-20" {
		t.Fatalf("digit entry: %q, want 2026-07-20", m.Value())
	}
}

func TestDatepicker_singleDigitMonthAutoCommits(t *testing.T) {
	m := datepicker.New(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), theme.Default)
	m = m.Update(key(tea.KeyRight)) // seg=month
	m = m.Update(digit("7"))        // 7 cannot start a 2-digit month -> commit 07, advance to day
	if m.Value() != "2026-07-15" {
		t.Fatalf("single-digit month: %q, want 2026-07-15", m.Value())
	}
}
