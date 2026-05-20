package cli

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
)

// pickerHost is an internal adapter that forwards to a writepicker.Model
// and intercepts DoneMsg → tea.Quit. The interesting branches are:
//   - Init delegates
//   - Update DoneMsg path stashes result + emits Quit
//   - Update non-DoneMsg path forwards to the inner picker
//   - View delegates

func TestPickerHost_DelegatesView(t *testing.T) {
	t.Parallel()
	h := pickerHost{inner: writepicker.New(true)}
	// View before any update should still produce a string.
	_ = h.View()
}

func TestPickerHost_InitDelegates(t *testing.T) {
	t.Parallel()
	h := pickerHost{inner: writepicker.New(true)}
	// Init may return nil or a textinput.Blink cmd — both are valid.
	_ = h.Init()
}

func TestPickerHost_DoneMsgYieldsQuit(t *testing.T) {
	t.Parallel()
	h := pickerHost{inner: writepicker.New(true)}
	result := writepicker.Result{Choice: writepicker.ChoiceDaily}
	next, cmd := h.Update(writepicker.DoneMsg{Result: result})
	if cmd == nil {
		t.Errorf("DoneMsg should produce a cmd (tea.Quit)")
	}
	nh, ok := next.(pickerHost)
	if !ok {
		t.Fatalf("Update should return pickerHost, got %T", next)
	}
	if nh.result.Choice != writepicker.ChoiceDaily {
		t.Errorf("result stashed: %+v", nh.result)
	}
}

func TestPickerHost_OtherMsgsForwardToInner(t *testing.T) {
	t.Parallel()
	h := pickerHost{inner: writepicker.New(true)}
	// A WindowSize message should fall through to the inner picker.
	next, _ := h.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if _, ok := next.(pickerHost); !ok {
		t.Errorf("Update should return pickerHost, got %T", next)
	}
}
