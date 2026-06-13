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
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "q"})
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
	m, _ = m.Update(tea.KeyPressMsg{Text: "2"})
	if _, ok := pressQ(t, m); !ok {
		t.Error("q on Woche tab must return tea.Quit")
	}
}

func TestQuit_FromHistoryListQuits(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyPressMsg{Text: "3"})
	if _, ok := pressQ(t, m); !ok {
		t.Error("q on History list must return tea.Quit")
	}
}

func TestQuit_FromFreiQuits(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyPressMsg{Text: "4"})
	if _, ok := pressQ(t, m); !ok {
		t.Error("q on Frei tab must return tea.Quit")
	}
}

func TestQuit_FromActionMenuListQuits(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyPressMsg{Text: ":"})
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
	m, _ = m.Update(tea.KeyPressMsg{Text: ":"})
	// Cursor is on first action which is Brief Wochenbericht.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	out := m.View().Content
	if !strings.Contains(out, "ausgabe-ziel") && !strings.Contains(out, "AUSGABE-ZIEL") {
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
	m, _ = m.Update(tea.KeyPressMsg{Text: "D"})
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
	m, _ = m.Update(tea.KeyPressMsg{Text: "t"})
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("precondition: tag dialog should be open")
	}
	updated, isQuit := pressQ(t, m)
	if isQuit {
		t.Error("q in tag textinput must NOT quit — the user is typing")
	}
	// Sanity: q ended up in the input — surface it via View().
	if !strings.Contains(updated.View().Content, "q") {
		t.Errorf("q should have landed in the tag input; got:\n%s", updated.View().Content)
	}
}

func TestQuit_DoesNotQuitInMenuRangeForm(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyPressMsg{Text: ":"})
	// Filter to "Export CSV", then Enter → Range form opens.
	for _, ch := range "csv" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(m.View().Content, "ZEITRAUM") {
		t.Fatalf("precondition: range form should be visible; got:\n%s", m.View().Content)
	}
	_, isQuit := pressQ(t, m)
	if isQuit {
		t.Error("q in range textinput must NOT quit")
	}
}

func TestQuit_DoesNotQuitInHistoryFilterDialog(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyPressMsg{Text: "3"})
	m, _ = m.Update(tea.KeyPressMsg{Text: "/"})
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("precondition: history filter dialog should be open")
	}
	if _, isQuit := pressQ(t, m); isQuit {
		t.Error("q in history filter input must NOT quit")
	}
}

func TestQuit_DoesNotQuitInFreiAddDialog(t *testing.T) {
	m := withSize(t, newModel(t))
	m, _ = m.Update(tea.KeyPressMsg{Text: "4"})
	m, _ = m.Update(tea.KeyPressMsg{Text: "a"})
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
	m, _ = m.Update(tea.KeyPressMsg{Text: ":"})
	// Filter to "Start" — narrows to the Korrektur action.
	for _, ch := range "Start" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(m.View().Content, "STARTZEIT") {
		t.Fatalf("precondition: correct form should be visible; got:\n%s", m.View().Content)
	}
	if _, isQuit := pressQ(t, m); isQuit {
		t.Error("q in correct HH:MM form must NOT quit")
	}
}

// — q does NOT quit when a full-screen overlay (note-viewer) is open —

// TestQuit_HeuteNoteViewerDoesNotQuit verifies that pressing `q` while
// the Heute inline note-viewer (opened with `o`) is active does NOT quit
// the app. The viewer advertises `q` as its close key; the worktime root
// must forward the key to the sub-model so the overlay closes itself
// (emitting markdown_overlay.ExitMsg) instead of tea.Quit.
func TestQuit_HeuteNoteViewerDoesNotQuit(t *testing.T) {
	r := newRig(t)
	r.noteReader.Bodies["daily-2026-05-01"] = "# Daily\n\nhello"
	if err := r.links.Add(r.clock.T, "daily-2026-05-01"); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	m := loadedHeute(t, r)

	// Open the inline note-viewer with `o`.
	m, cmd := m.Update(tea.KeyPressMsg{Text: "o"})
	m = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("precondition: note-viewer must be open (FilterActive=true)")
	}

	// Control: q on idle Heute DOES quit.
	idle := withSize(t, newModel(t))
	if _, ok := pressQ(t, idle); !ok {
		t.Fatal("control failed: q on idle Heute must produce tea.Quit")
	}

	// Subject: q while note-viewer is open must NOT quit.
	_, isQuit := pressQ(t, m)
	if isQuit {
		t.Error("q with Heute note-viewer open must NOT quit — it should close the overlay instead")
	}
}

// TestQuit_DrillNoteViewerDoesNotQuit mirrors TestQuit_HeuteNoteViewerDoesNotQuit
// for the History drill inline note-viewer (`o` key in drill mode).
func TestQuit_DrillNoteViewerDoesNotQuit(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	mon := isoMondayOf(r.clock.T)
	wed := mon.AddDate(0, 0, 2)
	preID := "notes/quit-test"
	if err := r.links.Add(wed, preID); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := loadedHistory(t, r)

	// Open drill on focused row (most recent = Wednesday per seedHistorySessions).
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("precondition: drill must be open (FilterActive=true)")
	}

	// Open the note-viewer inside the drill.
	m, cmd = m.Update(tea.KeyPressMsg{Text: "o"})
	m = drainCmd(t, m, cmd)
	if !strings.Contains(m.View().Content, "Note · "+preID) {
		t.Fatalf("precondition: note-viewer must be open (Note title not found); got:\n%s", m.View().Content)
	}

	// Subject: q while drill note-viewer is open must NOT quit.
	_, isQuit := pressQ(t, m)
	if isQuit {
		t.Error("q with drill note-viewer open must NOT quit — it should close the overlay instead")
	}
}
