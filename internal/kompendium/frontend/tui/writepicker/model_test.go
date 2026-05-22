package writepicker_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
)

func TestNew_HidesProjectWhenNotInRepo(t *testing.T) {
	t.Parallel()
	view := writepicker.New(false).View()
	if strings.Contains(view, "Projekt-Note") {
		t.Errorf("Project option should be hidden outside a repo:\n%s", view)
	}
	if !strings.Contains(view, "Daily-Note") || !strings.Contains(view, "Freie Note") {
		t.Errorf("Daily and Free options must always show:\n%s", view)
	}
}

func TestNew_ShowsProjectInRepo(t *testing.T) {
	t.Parallel()
	view := writepicker.New(true).View()
	if !strings.Contains(view, "Projekt-Note") {
		t.Errorf("Project option must show in repo:\n%s", view)
	}
}

func TestPicker_SelectsDaily(t *testing.T) {
	t.Parallel()
	m := writepicker.New(true)

	m, quit := drive(t, m, key("enter"))
	if !quit {
		t.Fatal("expected tea.Quit after Enter on Daily")
	}
	if m.Result().Choice != writepicker.ChoiceDaily {
		t.Errorf("Choice got %v, want Daily", m.Result().Choice)
	}
}

func TestPicker_SelectsProject(t *testing.T) {
	t.Parallel()
	m := writepicker.New(true)

	m, quit := drive(t, m, key("j"), key("enter"))
	if !quit {
		t.Fatal("expected tea.Quit after Enter on Project")
	}
	if m.Result().Choice != writepicker.ChoiceProject {
		t.Errorf("Choice got %v, want Project", m.Result().Choice)
	}
}

func TestPicker_SelectsFree_WithSlug(t *testing.T) {
	t.Parallel()
	m := writepicker.New(true)

	// Navigate to Free (third option), confirm to enter slug mode.
	m, quit := drive(t, m, key("j"), key("j"), key("enter"))
	if quit {
		t.Fatal("Enter on Free should NOT quit yet — slug entry follows")
	}
	if !strings.Contains(m.View(), "Slug für die neue freie Note") {
		t.Errorf("expected slug prompt, got:\n%s", m.View())
	}

	// Type a slug.
	for _, r := range "setup" {
		m, _ = sendOne(m, runeKey(r))
	}
	// Empty-trim Enter should still apply.
	m, quit = sendOne(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !quit {
		t.Fatal("Enter on a non-empty slug should quit")
	}
	if m.Result().Choice != writepicker.ChoiceFree || m.Result().Slug != "setup" {
		t.Errorf("result got %+v, want {Free setup}", m.Result())
	}
}

func TestPicker_RejectsEmptySlug(t *testing.T) {
	t.Parallel()
	m := writepicker.New(false)
	// Daily, Free → cursor on Free after one j.
	m, _ = sendOne(m, key("j"))
	m, _ = sendOne(m, key("enter"))

	// Press Enter without typing anything.
	_, quit := sendOne(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if quit {
		t.Error("empty slug must not quit; should re-prompt")
	}
}

func TestPicker_FreeSlug_BackspaceAndSpace(t *testing.T) {
	t.Parallel()
	m := writepicker.New(false)
	// Navigate to Free.
	m, _ = sendOne(m, key("j"))
	m, _ = sendOne(m, key("enter"))

	for _, r := range "abc" {
		m, _ = sendOne(m, runeKey(r))
	}
	m, _ = sendOne(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	m, _ = sendOne(m, tea.KeyPressMsg{Code: tea.KeySpace})
	m, _ = sendOne(m, runeKey('d'))

	if !strings.Contains(m.View(), "ab d") {
		t.Errorf("backspace/space wrong, view:\n%s", m.View())
	}
}

func TestPicker_CancelsOnQ(t *testing.T) {
	t.Parallel()
	m := writepicker.New(true)

	m, quit := drive(t, m, key("q"))
	if !quit {
		t.Fatal("q should quit")
	}
	if m.Result().Choice != writepicker.ChoiceCancel {
		t.Errorf("expected ChoiceCancel, got %v", m.Result().Choice)
	}
}

func TestPicker_CancelsOnEsc(t *testing.T) {
	t.Parallel()
	m := writepicker.New(true)

	m, quit := drive(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if !quit {
		t.Fatal("esc should quit")
	}
	if m.Result().Choice != writepicker.ChoiceCancel {
		t.Errorf("expected ChoiceCancel, got %v", m.Result().Choice)
	}
}

func TestPicker_CancelsFromSlugMode(t *testing.T) {
	t.Parallel()
	m := writepicker.New(false)
	m, _ = sendOne(m, key("j"))
	m, _ = sendOne(m, key("enter"))

	m, quit := sendOne(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if !quit {
		t.Fatal("esc in slug mode should cancel")
	}
	if m.Result().Choice != writepicker.ChoiceCancel {
		t.Errorf("expected ChoiceCancel from slug mode, got %v", m.Result().Choice)
	}
}

func TestPicker_CursorClampedAtEdges(t *testing.T) {
	t.Parallel()
	m := writepicker.New(false) // 2 options
	for range 5 {
		m, _ = sendOne(m, key("k"))
	}
	for range 5 {
		m, _ = sendOne(m, key("j"))
	}
	if !strings.Contains(m.View(), glyphsActive) {
		t.Errorf("cursor disappeared after edge navigation:\n%s", m.View())
	}
}

func TestPicker_QuittingViewIsEmpty(t *testing.T) {
	t.Parallel()
	m := writepicker.New(true)
	m, _ = sendOne(m, key("q"))
	if m.View() != "" {
		t.Errorf("quitting view should be empty, got %q", m.View())
	}
}

func TestPicker_IgnoresNonKeyMessages(t *testing.T) {
	t.Parallel()
	m := writepicker.New(true)
	m, _ = sendOne(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.View() == "" {
		t.Error("non-key messages should not blank the view")
	}
}

// glyphsActive ist die String-Form von glyphs.Active, hier dupliziert
// damit der Test nicht das interne glyphs-Package importieren muss
// (würde gegen die Test-Konvention verstoßen, dass Tests nur das
// Public-API des Packages-under-test berühren).
const glyphsActive = "▶"

// --- helpers ----------------------------------------------------------------

func key(s string) tea.KeyPressMsg   { return tea.KeyPressMsg{Text: s} }
func runeKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Text: string(r)} }

// sendOne forwards one message into the picker. Returns the updated
// model and a "done" boolean that's true iff the picker emitted a
// writepicker.DoneMsg via its returned cmd. Pre-DoneMsg refactor this
// looked for tea.QuitMsg; the picker now signals completion via the
// custom message so it can be embedded inside another bubbletea
// program (kompendium browse) without forcing a tea.Quit on the host.
func sendOne(m writepicker.Model, msg tea.Msg) (writepicker.Model, bool) {
	// writepicker.Update returns concrete Model under v2 (so it can
	// stay a sub-model without implementing tea.Model). No type
	// assertion needed.
	pm, cmd := m.Update(msg)
	if cmd == nil {
		return pm, false
	}
	if _, ok := cmd().(writepicker.DoneMsg); ok {
		return pm, true
	}
	return pm, false
}

func drive(t *testing.T, m writepicker.Model, msgs ...tea.Msg) (writepicker.Model, bool) {
	t.Helper()
	var quit bool
	for _, msg := range msgs {
		m, quit = sendOne(m, msg)
		if quit {
			break
		}
	}
	return m, quit
}
