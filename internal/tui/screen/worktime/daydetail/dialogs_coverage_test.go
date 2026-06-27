package daydetail_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/daydetail"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// TestDialogs_CapturesInputAndHints_WhileNachbuchenOpen exercises CapturesInput,
// nachbuchenHints (via KeyHints), and renderNachbuchen (via View) when the
// Nachbuchen dialog is open. These are all at 0% coverage.
func TestDialogs_CapturesInputAndHints_WhileNachbuchenOpen(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	f := &fakeAPI{
		projects: []domain.Node{{ID: "p1", Name: "Acme"}},
	}
	var r shell.Route = daydetail.NewRoute(f, theme.Default, day)

	// Load the day + project list via Init.
	r = drive(t, r, r.(interface{ Init() tea.Cmd }).Init())

	// Before opening a dialog: CapturesInput must be false.
	dr := r.(*daydetail.Route)
	if dr.CapturesInput() {
		t.Error("CapturesInput should be false when no dialog is open")
	}

	// Press 'n' to open Nachbuchen dialog; a project-load cmd is returned.
	r = press(t, r, keyRune('n'))
	dr = r.(*daydetail.Route)

	// After 'n', CapturesInput must be true (nachb is open).
	if !dr.CapturesInput() {
		t.Error("CapturesInput should be true while Nachbuchen dialog is open")
	}

	// KeyHints should return the Nachbuchen dialog hints.
	hints := dr.KeyHints()
	if len(hints) == 0 {
		t.Error("KeyHints should return non-empty hints for Nachbuchen dialog")
	}

	// View should render the dialog without panicking.
	out := dr.View(shell.Frame{Width: 80, Height: 30, Pal: theme.Default})
	if out == "" {
		t.Error("View during Nachbuchen dialog should return non-empty output")
	}
}

// TestDialogs_CapturesInputAndHints_WhileEditOpen exercises CapturesInput,
// editHints (via KeyHints), and renderEdit (via View) when the edit dialog is
// open. Seeds a completed session so the 'e' key can open the edit dialog.
func TestDialogs_CapturesInputAndHints_WhileEditOpen(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	s := day.Add(9 * time.Hour)
	e := day.Add(11 * time.Hour)
	f := &fakeAPI{sessions: []domain.WorkSession{{ID: "s1", Start: s, Stop: &e}}}
	var r shell.Route = daydetail.NewRoute(f, theme.Default, day)
	r = drive(t, r, r.(interface{ Init() tea.Cmd }).Init())

	// Press 'e' to open edit dialog on the first (and only) session.
	r = press(t, r, keyRune('e'))
	dr := r.(*daydetail.Route)

	if !dr.CapturesInput() {
		t.Error("CapturesInput should be true while edit dialog is open")
	}

	// KeyHints during edit must return the edit hints.
	hints := dr.KeyHints()
	if len(hints) == 0 {
		t.Error("KeyHints should return non-empty hints for edit dialog")
	}

	// View must render without panicking.
	out := dr.View(shell.Frame{Width: 80, Height: 30, Pal: theme.Default})
	if out == "" {
		t.Error("View during edit dialog should return non-empty output")
	}
}

// TestDialogs_CapturesInputAndHints_WhileDeleteOpen exercises CapturesInput,
// deleteHints (via KeyHints), and the delete UI when the delete confirmation
// dialog is open.
func TestDialogs_CapturesInputAndHints_WhileDeleteOpen(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	s := day.Add(9 * time.Hour)
	e := day.Add(11 * time.Hour)
	f := &fakeAPI{sessions: []domain.WorkSession{{ID: "s1", Start: s, Stop: &e}}}
	var r shell.Route = daydetail.NewRoute(f, theme.Default, day)
	r = drive(t, r, r.(interface{ Init() tea.Cmd }).Init())

	// Press 'd' to open delete dialog.
	r = press(t, r, keyRune('d'))
	dr := r.(*daydetail.Route)

	if !dr.CapturesInput() {
		t.Error("CapturesInput should be true while delete dialog is open")
	}

	// KeyHints during delete must return the delete hints.
	hints := dr.KeyHints()
	if len(hints) == 0 {
		t.Error("KeyHints should return non-empty hints for delete dialog")
	}

	// View must render without panicking.
	out := dr.View(shell.Frame{Width: 80, Height: 30, Pal: theme.Default})
	if out == "" {
		t.Error("View during delete dialog should return non-empty output")
	}
}

// TestNachbuchen_TriggersLoadProjectsCmd covers loadProjectsCmd (0% in coverage).
// When no projects are cached (empty fakeAPI.projects), pressing 'n' must fire
// loadProjectsCmd which fetches projects and opens the dialog on the result.
func TestNachbuchen_TriggersLoadProjectsCmd(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	// fakeAPI with NO projects in the initial cache: loadProjectsCmd will be called.
	f := &fakeAPI{projects: []domain.Node{}}
	var r shell.Route = daydetail.NewRoute(f, theme.Default, day)
	// Drive Init but DON'T seed projects yet — Init returns loadCmd which loads
	// sessions but also fills r.projects if the API returns some.  Since
	// f.projects is empty, r.projects stays empty after Init.
	r = drive(t, r, r.(interface{ Init() tea.Cmd }).Init())

	// Now inject a project so the async fetch can return it.
	f.projects = []domain.Node{{ID: "p2", Name: "Beta"}}

	// Press 'n': because r.projects is still empty the branch at line 254 fires
	// r.loadProjectsCmd(), which fetches projects and sends nachbuchenLoadProjectsMsg.
	// drive() executes the cmd, delivering the message, and the route then calls
	// openNachbuchen — CapturesInput becomes true.
	r = press(t, r, keyRune('n'))
	dr := r.(*daydetail.Route)
	if !dr.CapturesInput() {
		t.Error("after loadProjectsCmd completes, Nachbuchen dialog should be open (CapturesInput=true)")
	}
}
