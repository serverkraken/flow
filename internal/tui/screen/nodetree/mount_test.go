package nodetree

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeAll struct{ fakeTreeAPI }

func (f *fakeAll) GetNode(context.Context, string) (domain.Node, error)  { return domain.Node{}, nil }
func (f *fakeAll) Ancestors(context.Context, string) ([]domain.Node, error) { return nil, nil }
func (f *fakeAll) ListDocumentsScoped(context.Context, *string, ...string) ([]domain.Document, error) {
	return nil, nil
}
func (f *fakeAll) ListBindings(context.Context) ([]domain.ProjectBinding, error) { return nil, nil }
func (f *fakeAll) ListNodes(context.Context) ([]domain.Node, error)             { return nil, nil }
func (f *fakeAll) CreateNode(context.Context, CreateFields) (domain.Node, error) { return domain.Node{}, nil }
func (f *fakeAll) UpdateNode(context.Context, string, UpdateFields) (domain.Node, error) {
	return domain.Node{}, nil
}
func (f *fakeAll) SetNodeRate(context.Context, string, *int64, string) error { return nil }

func TestMountWithAPI_WiresFactories(t *testing.T) {
	t.Parallel()
	f := &fakeAll{}
	root := MountWithAPI(f, f, f, theme.Default, "u").(*Route)
	if root.detailFor == nil || root.formFor == nil {
		t.Fatal("Mount must wire detail + form factories")
	}
	// enter→detail and n→form must produce real child routes.
	root.all = []domain.Node{{ID: "e1", Kind: domain.KindEngagement, Name: "E"}}
	root.rebuild()
	if d := root.detailFor(root.rows[0].Node); d == nil {
		t.Fatal("detail factory returned nil")
	}
	if fm := root.formFor(nil); fm == nil {
		t.Fatal("form factory returned nil")
	}
	var _ shell.Route = root
}
