package projects

// White-box tests for worktimeProjectsModel — internal message handling.
// These live in package projects (not projects_test) so they can access
// unexported types like wpErrorMsg and worktimeProjectsModel directly.

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// TestWorktimeProjects_ErrorShowsToast verifies that a wpErrorMsg causes
// the model's toast slot to be set to a danger toast whose rendered text
// contains both the context and the error message.
//
// Before the fix this test fails because Update has no case wpErrorMsg:
// — the toast stays nil and the view shows nothing.
func TestWorktimeProjects_ErrorShowsToast(t *testing.T) {
	pal := theme.Load()
	m := newWorktimeProjects(pal, nil, nil, "")

	// Give the model a non-zero width so viewContent() renders.
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = sized.(worktimeProjectsModel)

	errMsg := wpErrorMsg{context: "archivieren", err: errors.New("server timeout")}
	updated, _ := m.Update(errMsg)

	wpm, ok := updated.(worktimeProjectsModel)
	if !ok {
		t.Fatalf("Update returned %T, want worktimeProjectsModel", updated)
	}

	if wpm.toast == nil {
		t.Fatal("toast should be set after wpErrorMsg, got nil")
	}

	if !wpm.toast.Visible() {
		t.Fatal("toast should be visible immediately after being set")
	}

	// Assert the toast's rendered text contains the context and error message.
	// We use toast.View() directly because viewContent() short-circuits to the
	// "nicht verfügbar" path when projects is nil (which omits the toast slot).
	// The toast text itself is the load-bearing check — it's what the user sees.
	toastText := ansi.Strip(wpm.toast.View())
	if !strings.Contains(toastText, "archivieren") {
		t.Errorf("toast text should contain context %q; got %q", "archivieren", toastText)
	}
	if !strings.Contains(toastText, "server timeout") {
		t.Errorf("toast text should contain error %q; got %q", "server timeout", toastText)
	}
}
