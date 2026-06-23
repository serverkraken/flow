package projects_test

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/projects"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeAPI struct{ ps []domain.Project }

func (f *fakeAPI) ListProjects(context.Context) ([]domain.Project, error) { return f.ps, nil }

func seed() []domain.Project {
	return []domain.Project{
		{ID: "p1", Slug: "aaa", Name: "Aaa", Status: domain.ProjectActive, Color: "blue", Glyph: "◆"},
		{ID: "p2", Slug: "bbb", Name: "Bbb", Status: domain.ProjectPaused},
		{ID: "p3", Slug: "ccc", Name: "Ccc", Status: domain.ProjectArchived},
	}
}

func drainInit(r *projects.Route) { // run Init's load cmd synchronously
	if cmd := r.Init(); cmd != nil {
		if msg := cmd(); msg != nil {
			nr, _ := r.Update(msg)
			*r = *nr.(*projects.Route)
		}
	}
}

func view(r *projects.Route) string { return r.View(shell.Frame{Width: 80, Height: 24}) }

// keyPress builds a tea.KeyPressMsg for a printable rune (matches week/route_test.go keyRune).
func keyPress(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Text: string(r)} }

// keyEnter returns a tea.KeyPressMsg for the Enter key (matches week/route_test.go keyEnter).
func keyEnter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }

func TestListShowsActivePausedHidesArchivedByDefault(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	drainInit(r)
	out := view(r)
	if !strings.Contains(out, "Aaa") || !strings.Contains(out, "Bbb") {
		t.Error("default view must list active + paused")
	}
	if strings.Contains(out, "Ccc") {
		t.Error("default view must hide archived")
	}
}

func TestStatusFilterCycleRevealsArchived(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	drainInit(r)
	// `]` advances the filter: default → archived
	nr, _ := r.Update(keyPress(']'))
	r = nr.(*projects.Route)
	if !strings.Contains(view(r), "Ccc") {
		t.Error("archived filter must reveal Ccc")
	}
}

func TestEnterPushesDetailWhenWired(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	pushed := false
	r.SetDetailFactory(func(p domain.Project) shell.Route { pushed = true; return nil })
	drainInit(r)
	_, cmd := r.Update(keyEnter())
	if cmd == nil {
		t.Fatal("enter should emit a command")
	}
	if _, ok := cmd().(shell.PushRouteMsg); !ok {
		t.Fatalf("enter should emit PushRouteMsg, got %T", cmd())
	}
	if !pushed {
		t.Error("detail factory should have been called with the selected project")
	}
}
