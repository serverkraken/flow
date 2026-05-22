package confirm_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/components/confirm"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
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

func TestNewDanger_RendersQuestion(t *testing.T) {
	t.Parallel()
	m := confirm.NewDanger("Wirklich löschen?", "Diese Sitzung wird unwiderruflich entfernt.", testPalette)
	v := m.View()
	if !strings.Contains(v, "Wirklich löschen?") {
		t.Errorf("danger view missing question: %q", v)
	}
	if !strings.Contains(v, "unwiderruflich") {
		t.Errorf("danger view missing detail: %q", v)
	}
}

func TestKeys_Exposes_DefaultMap(t *testing.T) {
	t.Parallel()
	km := confirm.New("Sure?", "", testPalette).Keys()
	if len(km.Confirm.Keys()) == 0 {
		t.Error("Confirm key binding has no keys")
	}
	if len(km.Cancel.Keys()) == 0 {
		t.Error("Cancel key binding has no keys")
	}
	// Help text is what an overlay surfaces — must be non-empty so
	// the binding is self-describing.
	if km.Confirm.Help().Key == "" || km.Confirm.Help().Desc == "" {
		t.Error("Confirm key has empty help")
	}
}
