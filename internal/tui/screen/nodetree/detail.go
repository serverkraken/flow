package nodetree

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/grammar"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// DetailAPI is the read surface the cockpit needs. *apiclient.Client satisfies it.
type DetailAPI interface {
	GetNode(ctx context.Context, id string) (domain.Node, error)
	Ancestors(ctx context.Context, id string) ([]domain.Node, error)
	ListDocumentsScoped(ctx context.Context, nodeID *string, tags ...string) ([]domain.Document, error)
	ListBindings(ctx context.Context) ([]domain.ProjectBinding, error)
}

var _ DetailAPI = (*apiclient.Client)(nil)

type detailLoadedMsg struct {
	node  domain.Node
	chain []domain.Node // leaf→root, as Ancestors returns
	docs  []domain.Document
	binds []domain.ProjectBinding
}

// DetailRoute is the node cockpit: name, kind badge, ancestor breadcrumb, rate
// (engagement), read-only bindings, assigned-docs count. Implements shell.Route.
type DetailRoute struct {
	api     DetailAPI
	pal     theme.Palette
	n       domain.Node
	data    detailLoadedMsg
	formFor func(*domain.Node) shell.Route
}

// NewDetailRoute constructs a read-only node cockpit route.
func NewDetailRoute(api DetailAPI, pal theme.Palette, n domain.Node) *DetailRoute {
	return &DetailRoute{api: api, pal: pal, n: n}
}

// SetFormFactory registers the factory that produces an edit Route for this node.
// A nil factory disables the 'e' keybinding.
func (r *DetailRoute) SetFormFactory(f func(*domain.Node) shell.Route) { r.formFor = f }

// Title implements shell.Route.
func (r *DetailRoute) Title() string { return r.n.Name }

// Init implements shell.Route — triggers the initial data load.
func (r *DetailRoute) Init() tea.Cmd { return r.loadCmd() }

func (r *DetailRoute) loadCmd() tea.Cmd {
	api, n := r.api, r.n
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		fresh, err := api.GetNode(ctx, n.ID)
		if err == nil {
			n = fresh
		}
		chain, _ := api.Ancestors(ctx, n.ID)
		id := n.ID
		docs, _ := api.ListDocumentsScoped(ctx, &id)
		allBinds, _ := api.ListBindings(ctx)
		var binds []domain.ProjectBinding
		for _, b := range allBinds {
			if b.NodeID == n.ID {
				binds = append(binds, b)
			}
		}
		return detailLoadedMsg{node: n, chain: chain, docs: docs, binds: binds}
	}
}

// Update implements shell.Route.
func (r *DetailRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case detailLoadedMsg:
		r.n, r.data = m.node, m
		return r, nil
	case shell.EventMsg:
		if m.Ev.Type == string(domain.EventNodeUpdated) || m.Ev.Type == string(domain.EventNodeMoved) {
			return r, r.loadCmd()
		}
		return r, nil
	case tea.KeyPressMsg:
		if grammar.Edit.Matches(m) && r.formFor != nil {
			cp := r.n
			return r, push(r.formFor(&cp))
		}
	}
	return r, nil
}

// View implements shell.Route.
func (r *DetailRoute) View(f shell.Frame) string { return renderDetailView(r, f) }

// KeyHints implements shell.Route.
func (r *DetailRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{grammar.Edit.Hint(), grammar.Back.Hint()}
}
