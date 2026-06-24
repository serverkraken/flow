package projects_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/projects"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// TestProjectRoute_EventMsg_ProjectEvent exercises the shell.EventMsg branch of
// Route.Update when the event is a project-related event (triggers reload).
func TestProjectRoute_EventMsg_ProjectEvent(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	drainInit(r)

	// Inject a project.created event (a project event) → should trigger reload cmd.
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventProjectCreated)}})
	if cmd == nil {
		t.Error("project event should trigger a reload cmd")
	}
}

// TestProjectRoute_EventMsg_UnrelatedEvent exercises the shell.EventMsg branch
// for a non-project event (returns nil cmd).
func TestProjectRoute_EventMsg_UnrelatedEvent(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	drainInit(r)

	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: "session.started"}})
	if cmd != nil {
		t.Error("non-project event should return nil cmd")
	}
}

// TestProjectRoute_FilterCycleBack exercises the WeekPrev (`[`) key which
// cycles the status filter backward. (WeekNext/`]` forward is already tested
// by TestStatusFilterCycleRevealsArchived.)
func TestProjectRoute_FilterCycleBack(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	drainInit(r)

	// Initially: default filter (0 = active+paused visible, archived hidden).
	// Pressing `[` cycles backward to 2 (all-statuses).
	nr, cmd := r.Update(tea.KeyPressMsg{Text: "["})
	r = nr.(*projects.Route)
	if cmd != nil {
		t.Error("filter cycle should not emit a cmd")
	}
	// Archived project "Ccc" should now be visible.
	out := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if out == "" {
		t.Error("View should not be empty after filter cycle")
	}
}

// TestProjectRoute_NewKey_WithNoFactory exercises the 'n' key when formFor is
// nil (the no-op path, returns nil cmd).
func TestProjectRoute_NewKey_WithNoFactory(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	drainInit(r)

	// formFor is not set → pressing 'n' should be a no-op.
	_, cmd := r.Update(tea.KeyPressMsg{Text: "n"})
	if cmd != nil {
		t.Error("n with nil formFor should return nil cmd")
	}
}

// TestProjectRoute_EnterKey_WithNoDetailFactory exercises the Enter key when
// detailFor is nil (the no-op path). Items are loaded but push returns nil.
func TestProjectRoute_EnterKey_WithNoDetailFactory(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	drainInit(r)

	// detailFor is not set → Enter should be a no-op.
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Error("Enter with nil detailFor should return nil cmd")
	}
}

// TestProjectRoute_KeyHints exercises Route.KeyHints (0% coverage).
func TestProjectRoute_KeyHints(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	hints := r.KeyHints()
	if hints == nil {
		t.Error("KeyHints should not be nil")
	}
}

// TestProjectRoute_Title exercises Route.Title.
func TestProjectRoute_Title(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	title := r.Title()
	if title == "" {
		t.Error("Title should not be empty")
	}
}

// TestMount_ConstructorSmoke covers projects.Mount (0% coverage).
// Mount takes a *apiclient.Client; we construct a client that points at a
// non-existent server (no requests are made by the test). The route returned
// must be non-nil.
func TestMount_ConstructorSmoke(t *testing.T) {
	client := apiclient.New("http://127.0.0.1:0", "test-token")
	route := projects.Mount(client, theme.Default, "msoent")
	if route == nil {
		t.Fatal("Mount should return a non-nil shell.Route")
	}
}
