package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestModel_TogglesDayOffView(t *testing.T) {
	m := New(nil, "msoent")
	// 'd' switches to the dayoff view.
	updated, _ := m.Update(tea.KeyPressMsg{Text: "d"})
	if !updated.(Model).showDayOffs {
		t.Fatal("expected dayoff view active after 'd'")
	}
	// 'esc' returns to worktime.
	back, _ := updated.(Model).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if back.(Model).showDayOffs {
		t.Fatal("expected worktime view after esc")
	}
}
