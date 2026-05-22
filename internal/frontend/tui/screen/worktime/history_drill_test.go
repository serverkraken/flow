package worktime_test

// Black-box tests for the history drill action handlers (a / D / Enter
// + the actual edit/add/delete flow). The existing
// TestHistory_DrillKey_NavigationAndDismiss covers j/k/g/G/esc; this
// suite drives the write-flow keys that exercise the history_edit
// dispatch helpers (openDrillEdit / openDrillAdd / openDrillDelete →
// submit → SessionWriter call → toast).

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
)

// drillOpened opens the History tab, drills into the focused row (the
// seedHistorySessions Mon entry) and returns the model in drill mode.
func drillOpened(t *testing.T, r rig) tea.Model {
	t.Helper()
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return drainCmd(t, m, cmd)
}

func TestDrill_AddDialog_OpensAndCancelsWithEsc(t *testing.T) {
	r := newRig(t)
	m := drillOpened(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "a"})
	m = drainCmd(t, m, cmd)
	out := m.View()
	if !strings.Contains(strings.ToLower(out), "neue session") {
		t.Errorf("drill add dialog should render its title, got:\n%s", out)
	}
	// Esc returns to the plain drill view (dialog cleared).
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if strings.Contains(strings.ToLower(m.View()), "neue session") {
		t.Errorf("Esc should close the add dialog")
	}
}

func TestDrill_EditDialog_OpensAndSubmits(t *testing.T) {
	r := newRig(t)
	m := drillOpened(t, r)
	// Enter on the focused drill row opens the edit dialog (we are
	// already on session index 0).
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	out := m.View()
	if !strings.Contains(strings.ToLower(out), "session bearbeiten") {
		t.Errorf("drill edit dialog should render its title, got:\n%s", out)
	}
	// Tab past stop/tag/note (3 tabs), Enter submits.
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)
	// After submit the drill view should be back without the dialog.
	if strings.Contains(strings.ToLower(m.View()), "session bearbeiten") {
		t.Errorf("after Enter submit the edit dialog should close, got:\n%s", m.View())
	}
}

func TestDrill_AddDialog_SubmitBadStartKeepsDialog(t *testing.T) {
	r := newRig(t)
	m := drillOpened(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "a"})
	m = drainCmd(t, m, cmd)
	// Backspace the prefilled start, type garbage.
	for i := 0; i < 6; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	for _, ch := range "abc" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	// Tab to stop, tag, note, then Enter (on note = last field → submit).
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)
	// Bad start should keep the dialog open.
	if !strings.Contains(strings.ToLower(m.View()), "neue session") {
		t.Errorf("invalid start should keep add dialog open, got:\n%s", m.View())
	}
}

func TestDrill_AddDialog_FillsValidEntry(t *testing.T) {
	r := newRig(t)
	m := drillOpened(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "a"})
	m = drainCmd(t, m, cmd)
	// Tab past start (already prefilled with last-session-stop), land on stop.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	// Type a stop value as +1h offset.
	for _, ch := range "+1h" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	// Tab to tag and note (leaving them blank).
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)
	// After successful submit dialog should be closed.
	if strings.Contains(strings.ToLower(m.View()), "neue session") {
		t.Errorf("successful submit should close the dialog, got:\n%s", m.View())
	}
}

func TestDrill_DeleteDialog_OpensAndCancels(t *testing.T) {
	r := newRig(t)
	m := drillOpened(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "D"})
	m = drainCmd(t, m, cmd)
	if !strings.Contains(m.View(), "löschen") {
		t.Errorf("drill delete dialog should render löschen, got:\n%s", m.View())
	}
	// "n" cancels the confirm.
	m, cmd = m.Update(tea.KeyPressMsg{Text: "n"})
	m = drainCmd(t, m, cmd)
	if strings.Contains(m.View(), "Session 1") && strings.Contains(strings.ToLower(m.View()), "löschen?") {
		t.Errorf("after `n` the confirm should be closed")
	}
}

func TestDrill_DeleteDialog_ConfirmDeletes(t *testing.T) {
	r := newRig(t)
	m := drillOpened(t, r)
	before := len(r.sessions.Sessions)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "D"})
	m = drainCmd(t, m, cmd)
	// "y" confirms.
	m, cmd = m.Update(tea.KeyPressMsg{Text: "y"})
	m = drainCmd(t, m, cmd)
	if len(r.sessions.Sessions) != before-1 {
		t.Errorf("after confirm delete, session count: got %d want %d", len(r.sessions.Sessions), before-1)
	}
	_ = m
	_ = time.Now // keep time import in case future tests need it
	_ = worktime.Model{}
	_ = domain.Session{}
}
