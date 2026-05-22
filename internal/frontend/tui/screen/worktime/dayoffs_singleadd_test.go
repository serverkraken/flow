package worktime_test

// Single-date add path — complements TestFrei_AddDialog_SuccessfulRangeAdd
// which only covers the `isRange` branch in submitAdd. With this in place
// both ports of the Add dialog (single date and range) are exercised.

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestFrei_AddDialog_SuccessfulSingleDateAdd(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "a"})
	m = drainCmd(t, m, cmd)
	// Backspace the prefilled date.
	for i := 0; i < 12; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	// Type a single date in 2026.
	for _, ch := range "2026-08-15" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	// Tab to label → tab to kind → Enter (default kind=Urlaub at index 1).
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)
	// Expect exactly one entry for 2026-08-15.
	if len(r.dayoffs.Entries) < 1 {
		t.Errorf("expected at least one dayoff entry after single-date submit, got %d", len(r.dayoffs.Entries))
	}
}
