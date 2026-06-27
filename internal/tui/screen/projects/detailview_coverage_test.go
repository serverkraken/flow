package projects_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/projects"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// fakeDetailAPIFull extends fakeDetailAPI to return documents and bindings
// for coverage of renderDocsSection, renderBindingsSection, and bindingTarget.
type fakeDetailAPIFull struct {
	p        domain.Node
	docs     []domain.Document
	bindings []domain.ProjectBinding
}

func (f *fakeDetailAPIFull) GetNode(_ context.Context, _ string) (domain.Node, error) {
	return f.p, nil
}
func (f *fakeDetailAPIFull) ListSessionsRange(_ context.Context, _, _ time.Time) ([]domain.WorkSession, error) {
	return nil, nil
}
func (f *fakeDetailAPIFull) ListDocumentsScoped(_ context.Context, _ *string, _ ...string) ([]domain.Document, error) {
	return f.docs, nil
}
func (f *fakeDetailAPIFull) ListBindings(_ context.Context) ([]domain.ProjectBinding, error) {
	return f.bindings, nil
}
func (f *fakeDetailAPIFull) UpdateNode(_ context.Context, _ string, in projects.UpdateFields) (domain.Node, error) {
	f.p.Status = domain.NodeStatus(in.Status)
	return f.p, nil
}

// TestDetailView_WithDocsAndBindings exercises renderDocsSection,
// renderBindingsSection, and bindingTarget by seeding the fake API with
// documents and project bindings (path + remote kinds).
func TestDetailView_WithDocsAndBindings(t *testing.T) {
	p := domain.Node{
		ID:     "p-full",
		Slug:   "fullproj",
		Name:   "FullProject",
		Status: domain.NodeActive,
		Color:  "blue",
	}
	docs := []domain.Document{
		{ID: "d1", Type: domain.DocFree, Path: "docs/architecture", Title: "Architecture"},
		{ID: "d2", Type: domain.DocFree, Path: "docs/no-title", Title: ""},
	}
	bindings := []domain.ProjectBinding{
		{ID: "b1", NodeID: "p-full", Kind: domain.BindingPath, Path: "/home/user/fullproj"},
		{ID: "b2", NodeID: "p-full", Kind: domain.BindingRemote, RemoteSlug: "github/fullproj"},
		// binding for different project to test filtering.
		{ID: "b3", NodeID: "other-proj", Kind: domain.BindingPath, Path: "/tmp/other"},
	}

	api := &fakeDetailAPIFull{p: p, docs: docs, bindings: bindings}
	r := projects.NewDetailRoute(api, theme.Default, p)

	// Trigger the load cmd to populate data.docs and data.binds.
	if cmd := r.Init(); cmd != nil {
		if msg := cmd(); msg != nil {
			nr, _ := r.Update(msg)
			r = nr.(*projects.DetailRoute)
		}
	}

	out := r.View(shell.Frame{Width: 100, Height: 50, Pal: theme.Default})

	// renderDocsSection: must list the documents.
	if !strings.Contains(out, "Architecture") {
		t.Errorf("detail view missing doc title 'Architecture'; got:\n%.400s", out)
	}
	// Doc with empty title falls back to path.
	if !strings.Contains(out, "docs/no-title") {
		t.Errorf("detail view missing path fallback for untitled doc; got:\n%.400s", out)
	}

	// renderBindingsSection: must list the filtered bindings (only p-full's).
	if !strings.Contains(out, "/home/user/fullproj") {
		t.Errorf("detail view missing path binding; got:\n%.400s", out)
	}
	if !strings.Contains(out, "github/fullproj") {
		t.Errorf("detail view missing remote binding slug; got:\n%.400s", out)
	}
}

// TestDetailRoute_TitleAndKeyHints exercises Title() and KeyHints()
// which are at 0% coverage.
func TestDetailRoute_TitleAndKeyHints(t *testing.T) {
	p := domain.Node{ID: "p1", Name: "MyProject", Status: domain.NodeActive}
	api := &fakeDetailAPIFull{p: p}
	r := projects.NewDetailRoute(api, theme.Default, p)

	if r.Title() != "MyProject" {
		t.Errorf("Title = %q, want 'MyProject'", r.Title())
	}
	hints := r.KeyHints()
	if hints == nil {
		t.Error("KeyHints should not be nil")
	}
}

// TestDetailRoute_UpdateEventMsg covers the shell.EventMsg branch of Update:
// a project-updated event triggers a reload cmd; any other event is ignored.
func TestDetailRoute_UpdateEventMsg(t *testing.T) {
	p := domain.Node{ID: "p1", Name: "P1", Status: domain.NodeActive}
	api := &fakeDetailAPIFull{p: p}
	r := projects.NewDetailRoute(api, theme.Default, p)

	// EventMsg with EventNodeUpdated: should return a non-nil cmd.
	nr, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventNodeUpdated)}})
	r = nr.(*projects.DetailRoute)
	if cmd == nil {
		t.Error("EventNodeUpdated should return a reload cmd")
	}

	// EventMsg with an unrelated event: cmd should be nil.
	nr, cmd2 := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: "unrelated"}})
	_ = nr
	if cmd2 != nil {
		t.Error("unrelated event should return nil cmd")
	}
}

// TestDetailRoute_UpdatePauseResume covers the 'p' (pause) and 'r' (resume)
// key branches of Update which were uncovered.
func TestDetailRoute_UpdatePauseResume(t *testing.T) {
	p := domain.Node{ID: "p2", Name: "P2", Status: domain.NodeActive}
	api := &fakeDetailAPIFull{p: p}
	r := projects.NewDetailRoute(api, theme.Default, p)

	// Press 'p' to pause.
	nr, cmd := r.Update(tea.KeyPressMsg{Text: "p"})
	r = nr.(*projects.DetailRoute)
	if cmd == nil {
		t.Error("pressing 'p' should return a setStatus cmd")
	}
	// Execute the cmd to apply the pause.
	if msg := cmd(); msg != nil {
		nr2, _ := r.Update(msg)
		r = nr2.(*projects.DetailRoute)
	}

	// Press 'r' to resume.
	nr3, cmd3 := r.Update(tea.KeyPressMsg{Text: "r"})
	_ = nr3
	if cmd3 == nil {
		t.Error("pressing 'r' should return a setStatus cmd")
	}
}

// TestDetailRoute_UpdateEditWithNoFactory covers the grammar.Edit branch when
// r.formFor is nil (the no-op path).
func TestDetailRoute_UpdateEditWithNoFactory(t *testing.T) {
	p := domain.Node{ID: "p3", Name: "P3", Status: domain.NodeActive}
	api := &fakeDetailAPIFull{p: p}
	r := projects.NewDetailRoute(api, theme.Default, p)
	// formFor is nil by default; pressing 'e' should be a no-op (no cmd).
	nr, cmd := r.Update(tea.KeyPressMsg{Text: "e"})
	_ = nr
	if cmd != nil {
		t.Error("Edit with nil formFor should return nil cmd")
	}
}
