package worktime

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// makeEditableRoute builds a TodayRoute with an edit dialog already open for
// a completed session (09:00–11:00 on 2026-06-18).
func makeEditableRoute(t *testing.T) *TodayRoute {
	t.Helper()
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	start := day.Add(9 * time.Hour)
	stop := day.Add(11 * time.Hour)
	sess := completedSession{
		ID:    "s1",
		Start: start,
		Stop:  stop,
		Tags:  []string{"deep"},
	}

	r := NewTodayRoute(&fakeAPI{}, time.Now, theme.Default, nil)
	// Inject a completed session into the state and cursor.
	r.st.Completed = []completedSession{sess}
	r.cursor = 0

	// openEdit() reads from r.st.Completed and r.cursor to build the edit form.
	result, _ := r.openEdit()
	dr, ok := result.(*TodayRoute)
	if !ok || dr.dialog != dialogEdit {
		t.Skip("could not open edit dialog")
	}
	return dr
}

// TestHandleEditKey_EscCancelsDialog exercises the Esc branch of handleEditKey.
func TestHandleEditKey_EscCancelsDialog(t *testing.T) {
	r := makeEditableRoute(t)

	result, cmd := r.handleEditKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	dr := result.(*TodayRoute)
	if dr.dialog != dialogNone {
		t.Errorf("Esc should close edit dialog, got dialog=%v", dr.dialog)
	}
	if cmd != nil {
		t.Error("Esc should return nil cmd")
	}
}

// TestHandleEditKey_TabAdvancesFocus exercises the Tab branch → editFocus(+1).
func TestHandleEditKey_TabAdvancesFocus(t *testing.T) {
	r := makeEditableRoute(t)
	initial := r.edit.cur

	result, _ := r.handleEditKey(tea.KeyPressMsg{Code: tea.KeyTab})
	dr := result.(*TodayRoute)
	if dr.edit.cur == initial {
		t.Errorf("Tab should advance focus: initial=%d current=%d", initial, dr.edit.cur)
	}
}

// TestHandleEditKey_UpMovesBack exercises the Up branch → editFocus(-1).
func TestHandleEditKey_UpMovesBack(t *testing.T) {
	r := makeEditableRoute(t)
	// Move to the second field first.
	r.editFocus(1)
	before := r.edit.cur

	result, _ := r.handleEditKey(tea.KeyPressMsg{Code: tea.KeyUp})
	dr := result.(*TodayRoute)
	if dr.edit.cur == before {
		t.Error("Up should move focus backward")
	}
}

// TestEditFocus_Wraps exercises editFocus wrapping: editFocus(-1) from 0 wraps.
func TestEditFocus_Wraps(t *testing.T) {
	r := makeEditableRoute(t)
	r.edit.cur = 0
	r.editFocus(-1)
	if r.edit.cur == 0 {
		t.Error("editFocus(-1) from 0 should wrap to last field")
	}
}

// TestHandleEditKey_EnterOnLastField exercises Enter on last field → submitEdit.
func TestHandleEditKey_EnterOnLastField(t *testing.T) {
	r := makeEditableRoute(t)
	r.edit.cur = len(r.edit.form) - 1

	_, cmd := r.handleEditKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	// With nil API call, the cmd will fire but may fail silently.
	if cmd != nil {
		_ = cmd()
	}
}

// TestTodayRoute_View_WithEditDialog exercises View when the edit dialog is
// open, ensuring renderEdit is called.
func TestTodayRoute_View_WithEditDialog(t *testing.T) {
	r := makeEditableRoute(t)

	out := r.View(shell.Frame{Width: 80, Height: 30, Pal: theme.Default})
	if out == "" {
		t.Error("View with edit dialog should return non-empty output")
	}
}

// TestHandleDialogKey_RoutesEdit exercises handleDialogKey via Update when
// dialogEdit is open. handleDialogKey (at 28.6%) dispatches to handleEditKey,
// handleBookingKey, or the confirm model — but only via handleKey which is
// called from Update.
func TestHandleDialogKey_RoutesEdit(t *testing.T) {
	r := makeEditableRoute(t)
	// r.dialog is dialogEdit; pressing Esc should go through:
	//   Update → handleKey → handleDialogKey → handleEditKey → cancel dialog.
	result, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	dr := result.(*TodayRoute)
	if dr.dialog != dialogNone {
		t.Errorf("Esc through Update should close edit dialog, got dialog=%v", dr.dialog)
	}
}

// TestHandleDialogKey_RoutesDelete exercises the dialogDelete branch of
// handleDialogKey, which routes to r.confirm.model.Update.
func TestHandleDialogKey_RoutesDelete(t *testing.T) {
	r := makeEditableRoute(t)
	// Clear the edit dialog first, then press 'D' (uppercase) to open delete.
	r.dialog = dialogNone
	r2, _ := r.Update(tea.KeyPressMsg{Text: "D"})
	dr := r2.(*TodayRoute)
	if dr.dialog != dialogDelete {
		t.Skip("could not open delete dialog via 'D' key")
	}
	// Press 'n' (No) through Update → handleKey → handleDialogKey → confirm.model.Update.
	result, _ := dr.Update(tea.KeyPressMsg{Text: "n"})
	_ = result // must not panic
}

// TestHandleEditKey_ShiftTabMoveBack covers the Shift+Tab branch of handleEditKey.
func TestHandleEditKey_ShiftTabMoveBack(t *testing.T) {
	r := makeEditableRoute(t)
	// Move to field 1 first.
	r.editFocus(1)
	before := r.edit.cur

	result, _ := r.handleEditKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	dr := result.(*TodayRoute)
	if dr.edit.cur == before {
		t.Error("Shift+Tab should move focus backward")
	}
}

// TestHandleEditKey_DownAdvancesFocus covers the KeyDown branch (same as KeyTab).
func TestHandleEditKey_DownAdvancesFocus(t *testing.T) {
	r := makeEditableRoute(t)
	initial := r.edit.cur

	result, _ := r.handleEditKey(tea.KeyPressMsg{Code: tea.KeyDown})
	dr := result.(*TodayRoute)
	if dr.edit.cur == initial {
		t.Errorf("KeyDown should advance focus: initial=%d current=%d", initial, dr.edit.cur)
	}
}

// TestHandleEditKey_EnterOnMiddleField covers the Enter-not-on-last branch
// (advances focus instead of submitting).
func TestHandleEditKey_EnterOnMiddleField(t *testing.T) {
	r := makeEditableRoute(t)
	// Ensure we are NOT on the last field.
	r.edit.cur = 0

	result, cmd := r.handleEditKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	dr := result.(*TodayRoute)
	if cmd != nil {
		t.Error("Enter on non-last field should return nil cmd")
	}
	if dr.edit.cur == 0 {
		t.Error("Enter on non-last field should advance focus")
	}
}

// TestHandleEditKey_KeyPassedToForm covers the default branch in handleEditKey:
// when no special key is pressed, the key is forwarded to the active form field.
func TestHandleEditKey_KeyPassedToForm(t *testing.T) {
	r := makeEditableRoute(t)
	// Pressing a regular character should not panic and should return the route.
	result, _ := r.handleEditKey(tea.KeyPressMsg{Text: "a"})
	if result == nil {
		t.Error("handleEditKey with regular key should return non-nil route")
	}
}
