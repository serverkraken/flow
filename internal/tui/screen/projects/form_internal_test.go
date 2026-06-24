package projects

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// TestFormUpdate_FormErrMsg exercises the formErrMsg case in Update
// (currently 0% from the external test package, which cannot construct the
// unexported type). Sending it via the internal test covers the branch.
func TestFormUpdate_FormErrMsg(t *testing.T) {
	r := NewFormRoute(nil, theme.Default, nil)

	// Inject a formErrMsg — only possible from within the package.
	result, cmd := r.Update(formErrMsg{err: "test error from internal test"})
	dr, ok := result.(*FormRoute)
	if !ok {
		t.Fatalf("Update(formErrMsg) returned %T, want *FormRoute", result)
	}
	if dr.err != "test error from internal test" {
		t.Errorf("err = %q, want %q", dr.err, "test error from internal test")
	}
	if cmd != nil {
		t.Error("Update(formErrMsg) should return nil cmd")
	}
}

// TestFormUpdate_ShiftTab exercises the Shift+Tab branch in Update.
func TestFormUpdate_ShiftTabInternal(t *testing.T) {
	r := NewFormRoute(nil, theme.Default, nil)
	r.focusIdx = 2 // move to a non-first field first

	result, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	dr := result.(*FormRoute)
	if dr.focusIdx == 2 {
		t.Error("Shift+Tab should move focus backward from field 2")
	}
}

// TestFormUpdate_KeyUp exercises the KeyUp branch in Update.
func TestFormUpdate_KeyUpInternal(t *testing.T) {
	r := NewFormRoute(nil, theme.Default, nil)
	r.focusIdx = 3 // start at field 3

	result, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	dr := result.(*FormRoute)
	if dr.focusIdx == 3 {
		t.Error("KeyUp should move focus backward from field 3")
	}
}

// TestFormUpdate_KeyDown exercises the KeyDown branch (same as Tab).
func TestFormUpdate_KeyDownInternal(t *testing.T) {
	r := NewFormRoute(nil, theme.Default, nil)
	r.focusIdx = 0 // start at field 0

	result, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	dr := result.(*FormRoute)
	if dr.focusIdx == 0 {
		t.Error("KeyDown should advance focus from field 0")
	}
}
