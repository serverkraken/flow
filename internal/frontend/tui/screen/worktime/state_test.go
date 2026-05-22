package worktime_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
)

// WithState restores the persisted tab (and sub-filter) so the
// sidekick parent can hand off "you were on History last time" without
// the user having to re-pick the tab. parseTabName is the inverse
// look-up the WithState path uses internally.

func TestWithState_RestoresHistoryTab(t *testing.T) {
	r := newRig(t)
	m := r.model
	restored := m.WithState("tab=history", 0)
	// Drive a WindowSizeMsg so the View() can produce something.
	updated, _ := restored.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := updated.View()
	if !strings.Contains(out, "History") {
		t.Errorf("WithState(tab=history) should land on History tab, got:\n%s", out)
	}
}

func TestWithState_RestoresWocheTab(t *testing.T) {
	r := newRig(t)
	m := r.model
	restored := m.WithState("tab=woche", 0)
	updated, _ := restored.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(updated.View(), "Woche") {
		t.Errorf("expected Woche tab after restore")
	}
}

func TestWithState_RestoresFreiTab(t *testing.T) {
	r := newRig(t)
	m := r.model
	restored := m.WithState("tab=frei", 0)
	updated, _ := restored.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(updated.View(), "Frei") {
		t.Errorf("expected Frei tab after restore")
	}
}

func TestWithState_RestoresHeuteAsDefault(t *testing.T) {
	r := newRig(t)
	// Unknown tab name → falls through, current stays at Heute.
	restored := r.model.WithState("tab=mystery", 0)
	updated, _ := restored.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(updated.View(), "Heute") {
		t.Errorf("unknown tab name should fall back to Heute")
	}
}

func TestWithState_RestoresHistoryWithSubFilter(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	// Sub-filter restoration depends on the sub-model implementing the
	// stateRestorer interface. We assert the tab restoration succeeds —
	// the sub-filter half is exercised through the broader integration
	// suite (TestHistory_FilterDialog_*).
	restored := r.model.WithState("tab=history|tag:deep", 0)
	updated, _ := restored.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	loaded := drainCmd(t, updated, updated.Init())
	if got := loaded.(worktime.Model).StateFilter(); !strings.HasPrefix(got, "tab=history") {
		t.Errorf("StateFilter after WithState should have tab=history prefix, got %q", got)
	}
}

func TestWithState_EmptyFilterStaysOnDefault(t *testing.T) {
	r := newRig(t)
	restored := r.model.WithState("", 0)
	updated, _ := restored.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(updated.View(), "Heute") {
		t.Errorf("empty filter should keep Heute as default")
	}
}
