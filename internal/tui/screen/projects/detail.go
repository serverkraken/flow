package projects

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientcheckout"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/gitworktree"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/grammar"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// UpdateFields is a type alias for apiclient.UpdateProjectFields so that
// tests and this package can reference it without importing apiclient directly.
// The alias ensures var _ DetailAPI = (*apiclient.Client)(nil) compiles.
type UpdateFields = apiclient.UpdateProjectFields

// DetailAPI is the narrow API surface the detail route needs. A fake
// implements it in tests; *apiclient.Client satisfies it in production
// (enforced by the compile assert in api.go, wired in Task 8).
type DetailAPI interface {
	GetProject(ctx context.Context, id string) (domain.Project, error)
	ListSessionsRange(ctx context.Context, since, until time.Time) ([]domain.WorkSession, error)
	ListDocumentsScoped(ctx context.Context, projectID *string, tags ...string) ([]domain.Document, error)
	ListBindings(ctx context.Context) ([]domain.ProjectBinding, error)
	UpdateProject(ctx context.Context, id string, in UpdateFields) (domain.Project, error)
}

// detailLoadedMsg is the internal message returned by loadCmd.
type detailLoadedMsg struct {
	p     domain.Project
	agg   worktimeAgg
	docs  []domain.Document
	binds []domain.ProjectBinding
	wts   []gitworktree.Worktree
	root  string // "" = not checked out on this machine
}

// detailReloadMsg triggers a fresh loadCmd after a status mutation.
type detailReloadMsg struct{}

// DetailRoute is the project cockpit: shows metadata, worktime aggregate,
// worktree panel (read-only), docs, and bindings. Implements shell.Route.
type DetailRoute struct {
	api  DetailAPI
	pal  theme.Palette
	p    domain.Project
	data detailLoadedMsg
	now  func() time.Time

	// formFor is injected by Task 8 wiring; nil until then.
	formFor func(*domain.Project) shell.Route
}

// NewDetailRoute builds an unloaded detail route. Call Init() to trigger the
// first data load.
func NewDetailRoute(api DetailAPI, pal theme.Palette, p domain.Project) *DetailRoute {
	return &DetailRoute{api: api, pal: pal, p: p, now: time.Now}
}

// SetFormFactory wires in the edit-form constructor (called by Task 8).
func (r *DetailRoute) SetFormFactory(f func(*domain.Project) shell.Route) { r.formFor = f }

// Title implements shell.Route.
func (r *DetailRoute) Title() string { return r.p.Name }

// Init implements shell.Route.
func (r *DetailRoute) Init() tea.Cmd { return r.loadCmd() }

// loadCmd fetches all cockpit data in one background call.
func (r *DetailRoute) loadCmd() tea.Cmd {
	api, p, now := r.api, r.p, r.now()
	return func() tea.Msg {
		// Refresh the project itself first.
		fresh, err := api.GetProject(context.Background(), p.ID)
		if err == nil {
			p = fresh
		}
		// Per-project worktime: pull a wide range and aggregate client-side
		// (no per-project backend usecase needed; mirrors the WebUI cockpit).
		sessions, _ := api.ListSessionsRange(context.Background(), now.AddDate(-5, 0, 0), now)
		agg := aggregate(p, sessions, now)
		// Project-scoped documents.
		pid := p.ID
		docs, _ := api.ListDocumentsScoped(context.Background(), &pid)
		// Bindings — filter to this project client-side.
		allBinds, _ := api.ListBindings(context.Background())
		var binds []domain.ProjectBinding
		for _, b := range allBinds {
			if b.ProjectID == p.ID {
				binds = append(binds, b)
			}
		}
		// Worktree panel (device-local, read-only).
		var wts []gitworktree.Worktree
		var root string
		if reg, _ := clientcheckout.Load(); true {
			if rt, ok := reg.Get(p.Slug); ok {
				root = rt
				wts, _ = gitworktree.List(rt)
			}
		}
		return detailLoadedMsg{p: p, agg: agg, docs: docs, binds: binds, wts: wts, root: root}
	}
}

// setStatusCmd applies a single status transition (full-replace UpdateProject,
// mirroring the WebUI handleWebProjectStatus) then triggers a reload.
func (r *DetailRoute) setStatusCmd(status string) tea.Cmd {
	api, p := r.api, r.p
	return func() tea.Msg {
		_, _ = api.UpdateProject(context.Background(), p.ID, UpdateFields{
			Name:        p.Name,
			Slug:        p.Slug,
			Color:       p.Color,
			Glyph:       p.Glyph,
			Description: p.Description,
			UpstreamGit: p.UpstreamGit,
			Status:      status,
		})
		return detailReloadMsg{}
	}
}

// Update implements shell.Route.
func (r *DetailRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case detailLoadedMsg:
		r.p, r.data = m.p, m
		return r, nil

	case detailReloadMsg:
		return r, r.loadCmd()

	case shell.EventMsg:
		if m.Ev.Type == string(domain.EventProjectUpdated) {
			return r, r.loadCmd()
		}
		return r, nil

	case tea.KeyPressMsg:
		switch {
		case grammar.Edit.Matches(m):
			if r.formFor != nil {
				pCopy := r.p
				return r, push(r.formFor(&pCopy))
			}
		case keyIs(m, 'p'):
			return r, r.setStatusCmd("paused")
		case keyIs(m, 'r'):
			return r, r.setStatusCmd("active")
		case keyIs(m, 'a'):
			return r, r.setStatusCmd("archived")
		}
	}
	return r, nil
}

// View implements shell.Route — rendering is split to detailview.go to keep
// this file under the ~200-line no-monolith threshold.
func (r *DetailRoute) View(f shell.Frame) string { return renderDetailView(r, f) }

// KeyHints implements shell.Route.
func (r *DetailRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		grammar.Edit.Hint(),
		{Key: "p/r/a", Desc: "Pausieren/Fortsetzen/Archivieren"},
		grammar.Back.Hint(),
	}
}

// keyIs reports whether m is an unmodified printable key matching r.
func keyIs(m tea.KeyPressMsg, r rune) bool {
	return m.Mod == 0 && m.Text == string(r)
}
