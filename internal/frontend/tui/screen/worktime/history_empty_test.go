package worktime_test

// Empty-history list-mode key tests — exercises the `if n := len(records); n > 0`
// guard branches in handleListKey when no records are loaded. The
// existing seeded tests only cover the "have records" path.

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHistory_EmptyList_NavigationKeysSafe(t *testing.T) {
	r := newRig(t)
	// No seedHistorySessions → empty record set.
	m := loadedHistory(t, r)
	// j, k, g, G, Enter on an empty list must not panic.
	for _, k := range []string{"j", "k", "g", "G"} {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
	// Enter on empty list should not open a drill.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)
}

func TestHistory_EmptyList_VToHeatmap(t *testing.T) {
	r := newRig(t)
	m := loadedHistory(t, r)
	// v switches to heatmap mode; with no records, heatmapTodayCell is used.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	_ = m.View()
}

func TestHistory_EmptyList_TResetsFilter(t *testing.T) {
	r := newRig(t)
	m := loadedHistory(t, r)
	// T resets filter; on empty list listCur stays at 0.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	_ = m
}
