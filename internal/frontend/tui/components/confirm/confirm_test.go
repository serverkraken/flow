package confirm_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/components/confirm"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
)

var testPalette = theme.Load()

func TestNew_ViewContainsQuestion(t *testing.T) {
	t.Parallel()
	m := confirm.New("Delete file?", "", testPalette)
	if !strings.Contains(m.View(), "Delete file?") {
		t.Error("view missing question text")
	}
}

func TestNew_ViewContainsDetail(t *testing.T) {
	t.Parallel()
	m := confirm.New("Delete?", "session 08:00 → 12:00", testPalette)
	if !strings.Contains(m.View(), "session 08:00") {
		t.Error("view missing detail text")
	}
}

func TestUpdate_YConfirms(t *testing.T) {
	t.Parallel()
	m := confirm.New("Sure?", "", testPalette)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected command from 'y' key")
	}
	msg := cmd()
	result, ok := msg.(confirm.ResultMsg)
	if !ok {
		t.Fatalf("expected ResultMsg, got %T", msg)
	}
	if !result.Confirmed {
		t.Error("expected Confirmed=true for 'y'")
	}
}

func TestUpdate_EnterConfirms(t *testing.T) {
	t.Parallel()
	m := confirm.New("Sure?", "", testPalette)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter key")
	}
	msg := cmd()
	result, ok := msg.(confirm.ResultMsg)
	if !ok {
		t.Fatalf("expected ResultMsg, got %T", msg)
	}
	if !result.Confirmed {
		t.Error("expected Confirmed=true for Enter")
	}
}

func TestUpdate_NDenies(t *testing.T) {
	t.Parallel()
	m := confirm.New("Sure?", "", testPalette)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil {
		t.Fatal("expected command from 'n' key")
	}
	msg := cmd()
	result, ok := msg.(confirm.ResultMsg)
	if !ok {
		t.Fatalf("expected ResultMsg, got %T", msg)
	}
	if result.Confirmed {
		t.Error("expected Confirmed=false for 'n'")
	}
}

func TestUpdate_EscDenies(t *testing.T) {
	t.Parallel()
	m := confirm.New("Sure?", "", testPalette)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected command from Esc key")
	}
	msg := cmd()
	result, ok := msg.(confirm.ResultMsg)
	if !ok {
		t.Fatalf("expected ResultMsg, got %T", msg)
	}
	if result.Confirmed {
		t.Error("expected Confirmed=false for Esc")
	}
}

func TestUpdate_UnknownKeyNoOp(t *testing.T) {
	t.Parallel()
	m := confirm.New("Sure?", "", testPalette)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Error("expected nil command for unhandled key")
	}
}
