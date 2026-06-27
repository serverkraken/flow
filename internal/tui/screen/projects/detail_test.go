package projects_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/projects"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeDetailAPI struct {
	p        domain.Node
	sessions []domain.WorkSession
}

func (f *fakeDetailAPI) GetNode(context.Context, string) (domain.Node, error) { return f.p, nil }
func (f *fakeDetailAPI) ListSessionsRange(context.Context, time.Time, time.Time) ([]domain.WorkSession, error) {
	return f.sessions, nil
}
func (f *fakeDetailAPI) ListDocumentsScoped(context.Context, *string, ...string) ([]domain.Document, error) {
	return nil, nil
}
func (f *fakeDetailAPI) ListBindings(context.Context) ([]domain.ProjectBinding, error) { return nil, nil }
func (f *fakeDetailAPI) UpdateNode(_ context.Context, _ string, in projects.UpdateFields) (domain.Node, error) {
	f.p.Status = domain.NodeStatus(in.Status)
	return f.p, nil
}

func detailView(r *projects.DetailRoute) string { return r.View(shell.Frame{Width: 80, Height: 30}) }

func TestDetailRendersCockpit(t *testing.T) {
	p := domain.Node{
		ID: "p1", Slug: "flow", Name: "Flow", Status: domain.NodePaused,
		Description: "# Notiz\nhallo", UpstreamGit: "git@github.com:acme/flow.git", Color: "blue",
	}
	api := &fakeDetailAPI{p: p}
	r := projects.NewDetailRoute(api, theme.Default, p)
	if cmd := r.Init(); cmd != nil {
		if msg := cmd(); msg != nil {
			nr, _ := r.Update(msg)
			r = nr.(*projects.DetailRoute)
		}
	}
	out := detailView(r)
	for _, want := range []string{"Flow", "Notiz", "nicht ausgecheckt", "pausiert"} {
		if !strings.Contains(out, want) {
			t.Errorf("cockpit missing %q\n%s", want, out)
		}
	}
}

func TestDetailStatusActionArchives(t *testing.T) {
	p := domain.Node{ID: "p1", Slug: "flow", Name: "Flow", Status: domain.NodeActive}
	api := &fakeDetailAPI{p: p}
	r := projects.NewDetailRoute(api, theme.Default, p)
	// `a` archives via UpdateNode then reload
	nr, cmd := r.Update(keyPress('a'))
	r = nr.(*projects.DetailRoute)
	if cmd != nil {
		if msg := cmd(); msg != nil {
			_, _ = r.Update(msg)
		}
	}
	if api.p.Status != domain.NodeArchived {
		t.Errorf("status action did not archive: %s", api.p.Status)
	}
}
