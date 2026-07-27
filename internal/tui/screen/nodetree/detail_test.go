package nodetree

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeDetailAPI struct {
	node  domain.Node
	chain []domain.Node
	docs  []domain.Document
	binds []domain.ProjectBinding
}

func (f *fakeDetailAPI) GetNode(_ context.Context, _ string) (domain.Node, error) { return f.node, nil }
func (f *fakeDetailAPI) Ancestors(_ context.Context, _ string) ([]domain.Node, error) {
	return f.chain, nil
}
func (f *fakeDetailAPI) ListDocumentsScoped(_ context.Context, _ *string, _ ...string) ([]domain.Document, error) {
	return f.docs, nil
}
func (f *fakeDetailAPI) ListBindings(_ context.Context) ([]domain.ProjectBinding, error) {
	return f.binds, nil
}

func TestDetail_RendersCockpit(t *testing.T) {
	t.Parallel()
	rate := domain.Money{Amount: 9500, Currency: "EUR"}
	f := &fakeDetailAPI{
		node:  domain.Node{ID: "e1", Kind: domain.KindEngagement, Name: "RTL Extern", Rate: &rate},
		chain: []domain.Node{{ID: "e1", Kind: domain.KindEngagement, Name: "RTL Extern"}},
		docs:  []domain.Document{{ID: "d1", Title: "Spec"}},
	}
	r := NewDetailRoute(f, theme.Default, f.node)
	r.Update(detailLoadedMsg{node: f.node, chain: f.chain, docs: f.docs})
	out := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	for _, want := range []string{"RTL Extern", "ENGAGEMENT", "95.00 EUR", "Dokumente (1)"} {
		if !contains(out, want) {
			t.Errorf("cockpit missing %q in:\n%s", want, out)
		}
	}
}

func TestDetail_BreadcrumbRootToLeaf(t *testing.T) {
	t.Parallel()
	f := &fakeDetailAPI{
		node: domain.Node{ID: "r1", Kind: domain.KindRepo, Name: "flow"},
		// Ancestors returns leaf→root; view must render root→leaf.
		chain: []domain.Node{
			{ID: "r1", Kind: domain.KindRepo, Name: "flow"},
			{ID: "e1", Kind: domain.KindEngagement, Name: "Privat"},
		},
	}
	r := NewDetailRoute(f, theme.Default, f.node)
	r.Update(detailLoadedMsg{node: f.node, chain: f.chain})
	out := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !contains(out, "Privat › flow") {
		t.Errorf("breadcrumb wrong:\n%s", out)
	}
}

func TestDetail_EditPushesForm(t *testing.T) {
	t.Parallel()
	f := &fakeDetailAPI{node: domain.Node{ID: "e1", Kind: domain.KindEngagement, Name: "E"}}
	r := NewDetailRoute(f, theme.Default, f.node)
	r.SetFormFactory(func(*domain.Node) shell.Route { return nil })
	_, cmd := r.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if cmd == nil {
		t.Fatal("e must push edit form")
	}
	if _, ok := cmd().(shell.PushRouteMsg); !ok {
		t.Fatal("e cmd must emit PushRouteMsg")
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || (len(s) >= len(sub) && indexOf2(s, sub) >= 0) }
func indexOf2(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
