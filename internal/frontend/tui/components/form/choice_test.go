package form_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

var testPalette = theme.Load()

func choices() []form.Choice {
	return []form.Choice{
		{Label: "feat", Value: "feat"},
		{Label: "fix", Value: "fix"},
		{Label: "docs", Value: "docs"},
	}
}

func TestChoiceModel_InitialCursorZero(t *testing.T) {
	t.Parallel()
	m := form.NewChoice(choices(), 40, testPalette)
	if m.Cursor() != 0 {
		t.Errorf("initial cursor = %d, want 0", m.Cursor())
	}
}

func TestChoiceModel_JMovesCursorDown(t *testing.T) {
	t.Parallel()
	m := form.NewChoice(choices(), 40, testPalette)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.Cursor() != 1 {
		t.Errorf("cursor after j = %d, want 1", m.Cursor())
	}
}

func TestChoiceModel_KMovesCursorUp(t *testing.T) {
	t.Parallel()
	m := form.NewChoice(choices(), 40, testPalette)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.Cursor() != 0 {
		t.Errorf("cursor after j+k = %d, want 0", m.Cursor())
	}
}

func TestChoiceModel_CursorClampsAtBounds(t *testing.T) {
	t.Parallel()
	m := form.NewChoice(choices(), 40, testPalette)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.Cursor() != 0 {
		t.Errorf("cursor should clamp at 0, got %d", m.Cursor())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.Cursor() != 2 {
		t.Errorf("cursor should clamp at 2, got %d", m.Cursor())
	}
}

func TestChoiceModel_EnterSendsSelected(t *testing.T) {
	t.Parallel()
	m := form.NewChoice(choices(), 40, testPalette)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter")
	}
	msg := cmd()
	sel, ok := msg.(form.SelectedMsg)
	if !ok {
		t.Fatalf("expected SelectedMsg, got %T", msg)
	}
	if sel.Index != 1 || sel.Value != "fix" {
		t.Errorf("got {%d, %q}, want {1, fix}", sel.Index, sel.Value)
	}
}

func TestChoiceModel_EscSendsCancelled(t *testing.T) {
	t.Parallel()
	m := form.NewChoice(choices(), 40, testPalette)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected command from Esc")
	}
	msg := cmd()
	if _, ok := msg.(form.CancelledMsg); !ok {
		t.Fatalf("expected CancelledMsg, got %T", msg)
	}
}

func TestChoiceModel_ViewContainsLabels(t *testing.T) {
	t.Parallel()
	m := form.NewChoice(choices(), 40, testPalette)
	v := m.View()
	for _, c := range choices() {
		if !strings.Contains(v, c.Label) {
			t.Errorf("view missing label %q", c.Label)
		}
	}
}
