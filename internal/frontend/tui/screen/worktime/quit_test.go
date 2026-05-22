// Tests for the universal `q`-quits-from-anywhere shortcut introduced
// alongside Slice E. q must return tea.Quit from every Worktime sub-
// surface UNLESS a textinput is currently focused — in those cases
// (tag / note / range / HH:MM forms) q is a literal letter the user
// is typing into the field.

package worktime_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
)

// pressQ sends a `q` keystroke to the model and reports whether the
// returned tea.Cmd resolves to tea.QuitMsg.
func pressQ(t *testing.T, m tea.Model) (tea.Model, bool) {
	t.Helper()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		return updated, false
	}
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	return updated, ok
}

// withSize runs the WindowSizeMsg through the model so View() works
// and any sub-model that captures the width during init can.
func withSize(t *testing.T, m tea.Model) tea.Model {
	t.Helper()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return updated
}

// — q quits from non-text-input surfaces —

func TestQuit_FromHeuteIdleQuits(t *testing.T) {
	m := withSize(t, newModel(t))
	if _, ok := pressQ(t, m); !ok {
		t.Error("q on idle Heute must return tea.Quit")
	}
}

func TestQuit_FromWocheQuits(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if _, ok := pressQ(t, m); !ok {
		t.Error("q on Woche tab must return tea.Quit")
	}
}

func TestQuit_FromHistoryListQuits(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	if _, ok := pressQ(t, m); !ok {
		t.Error("q on History list must return tea.Quit")
	}
}

func TestQuit_FromFreiQuits(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	if _, ok := pressQ(t, m); !ok {
		t.Error("q on Frei tab must return tea.Quit")
	}
}

func TestQuit_FromActionMenuListQuits(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("precondition: menu must be open")
	}
	if _, ok := pressQ(t, m); !ok {
		t.Error("q in action menu list must return tea.Quit (not extend the filter)")
	}
}

// — q quits from non-text dialogs (delete confirm, target picker, etc.) —

func TestQuit_FromTargetPickerQuits(t *testing.T) {
	r := newRig(t)
	m := withSize(t, r.model)
	// Open menu, navigate to "Brief Wochenbericht", Enter → Target picker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	// Cursor is on first action which is Brief Wochenbericht.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	out := m.View()
	if !strings.Contains(out, "output-ziel") && !strings.Contains(out, "OUTPUT-ZIEL") {
		t.Fatalf("precondition: target sub-picker must be visible; got:\n%s", out)
	}
	if _, ok := pressQ(t, m); !ok {
		t.Error("q in target picker must return tea.Quit")
	}
}

func TestQuit_FromHeuteDeleteConfirmQuits(t *testing.T) {
	r := newRig(t)
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	r.sessions.Sessions = []domain.Session{{
		Date: today, Start: today.Add(9 * time.Hour),
		Stop: today.Add(10 * time.Hour), Elapsed: time.Hour,
	}}
	m := withSize(t, r.model)
	m = drainCmd(t, m, m.Init())
	// Press D to open delete confirm.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")})
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("precondition: delete confirm should be open")
	}
	if _, ok := pressQ(t, m); !ok {
		t.Error("q in delete confirm dialog must return tea.Quit (no text input there)")
	}
}

// — q does NOT quit when a textinput is focused —

func TestQuit_DoesNotQuitInHeuteTagDialog(t *testing.T) {
	r := newRig(t)
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	r.sessions.Sessions = []domain.Session{{
		Date: today, Start: today.Add(9 * time.Hour),
		Stop: today.Add(10 * time.Hour), Elapsed: time.Hour,
	}}
	m := withSize(t, r.model)
	m = drainCmd(t, m, m.Init())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("precondition: tag dialog should be open")
	}
	updated, isQuit := pressQ(t, m)
	if isQuit {
		t.Error("q in tag textinput must NOT quit — the user is typing")
	}
	// Sanity: q ended up in the input — surface it via View().
	if !strings.Contains(updated.View(), "q") {
		t.Errorf("q should have landed in the tag input; got:\n%s", updated.View())
	}
}

func TestQuit_DoesNotQuitInMenuRangeForm(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	// Filter to "Export CSV", then Enter → Range form opens.
	for _, ch := range "csv" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.View(), "RANGE") {
		t.Fatalf("precondition: range form should be visible; got:\n%s", m.View())
	}
	_, isQuit := pressQ(t, m)
	if isQuit {
		t.Error("q in range textinput must NOT quit")
	}
}

func TestQuit_DoesNotQuitInHistoryFilterDialog(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("precondition: history filter dialog should be open")
	}
	if _, isQuit := pressQ(t, m); isQuit {
		t.Error("q in history filter input must NOT quit")
	}
}

func TestQuit_DoesNotQuitInFreiAddDialog(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("precondition: frei add dialog should be open")
	}
	if _, isQuit := pressQ(t, m); isQuit {
		t.Error("q in frei add input must NOT quit")
	}
}

func TestQuit_DoesNotQuitInMenuCorrectForm(t *testing.T) {
	r := newRig(t)
	// Seed an active session so the Korrektur predicate passes.
	start := r.clock.T.Add(-30 * time.Minute)
	r.active.Active = &start
	m := withSize(t, r.model)
	m = drainCmd(t, m, m.Init())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	// Filter to "Start" — narrows to the Korrektur action.
	for _, ch := range "Start" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.View(), "STARTZEIT") {
		t.Fatalf("precondition: correct form should be visible; got:\n%s", m.View())
	}
	if _, isQuit := pressQ(t, m); isQuit {
		t.Error("q in correct HH:MM form must NOT quit")
	}
}
