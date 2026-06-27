// Package projects is the "Projekte" tab root: a list of projects with a
// status filter (active+paused / archived / all), cursor navigation, and
// SSE-driven live reload. detail and form child routes are injected via
// factory setters so this package stays free of import cycles until Task 8.
package projects

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/grammar"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/listnav"
)

// statusFilter is the active filter applied to the loaded project list.
type statusFilter int

const (
	filterActivePaused statusFilter = iota // default: active + paused
	filterArchived
	filterAll
)

func (f statusFilter) label() string {
	switch f {
	case filterArchived:
		return "archiviert"
	case filterAll:
		return "alle"
	default:
		return "aktiv + pausiert"
	}
}

// loadedMsg is the internal message delivered when the API call completes.
type loadedMsg struct {
	ps  []domain.Node
	err error
}

// Route is the projects list leaf route (the "Projekte" tab root).
type Route struct {
	api    ProjectsAPI
	pal    theme.Palette
	user   string
	all    []domain.Node // unfiltered, as loaded from the server
	shown  []domain.Node // view-filtered subset
	cur    listnav.Cursor
	filter statusFilter
	loaded bool
	err    error

	// detailFor and formFor are injected by Task 8's wiring. Until then they
	// are nil and enter/n are silent no-ops.
	detailFor func(domain.Node) shell.Route
	formFor   func(*domain.Node) shell.Route // nil ptr → create; non-nil ptr → edit
}

// NewRoute returns an unloaded projects list route. Call Init() to trigger
// the first data load.
func NewRoute(api ProjectsAPI, pal theme.Palette, user string) *Route {
	return &Route{api: api, pal: pal, user: user, cur: listnav.New()}
}

// SetDetailFactory wires in the detail-route constructor (called by Task 8).
func (r *Route) SetDetailFactory(f func(domain.Node) shell.Route) { r.detailFor = f }

// SetFormFactory wires in the form-route constructor (called by Task 8).
func (r *Route) SetFormFactory(f func(*domain.Node) shell.Route) { r.formFor = f }

// Title implements shell.Route.
func (r *Route) Title() string { return "Projekte" }

// Init implements shell.Route.
func (r *Route) Init() tea.Cmd { return r.loadCmd() }

func (r *Route) loadCmd() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ps, err := api.ListNodes(context.Background())
		return loadedMsg{ps: ps, err: err}
	}
}

// Update implements shell.Route.
func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded = true
		r.all, r.err = m.ps, m.err
		r.applyFilter()
		return r, nil

	case shell.EventMsg:
		if isProjectEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil

	case tea.KeyPressMsg:
		switch {
		case grammar.WeekNext.Matches(m): // `]` — cycle filter forward
			r.filter = (r.filter + 1) % 3
			r.applyFilter()
			return r, nil

		case grammar.WeekPrev.Matches(m): // `[` — cycle filter back
			r.filter = (r.filter + 2) % 3
			r.applyFilter()
			return r, nil

		case grammar.New.Matches(m): // `n` — new project
			if r.formFor != nil {
				return r, push(r.formFor(nil))
			}
			return r, nil

		case grammar.Open.Matches(m): // enter → open detail
			if r.detailFor != nil && len(r.shown) > 0 {
				return r, push(r.detailFor(r.shown[r.cur.Index()]))
			}
			return r, nil
		}

		// Arrow / Home / End / PgUp / PgDn cursor navigation.
		if c, ok := r.cur.Handle(m, len(r.shown), 0); ok {
			r.cur = c
			return r, nil
		}
	}
	return r, nil
}

// View implements shell.Route — delegates to the rendering helpers in view.go.
func (r *Route) View(f shell.Frame) string { return renderView(r, f) }

// KeyHints implements shell.Route.
func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		grammar.Open.Hint(),
		grammar.New.Hint(),
		{Key: "[ ]", Desc: "Filter"},
		grammar.MoveUp.Hint(),
	}
}

// push wraps a child Route in a PushRouteMsg command.
func push(child shell.Route) tea.Cmd {
	return func() tea.Msg { return shell.PushRouteMsg{Route: child} }
}

// applyFilter rebuilds r.shown from r.all according to r.filter and
// clamps the cursor so it stays within bounds.
func (r *Route) applyFilter() {
	r.shown = r.shown[:0]
	for _, p := range r.all {
		switch r.filter {
		case filterAll:
			r.shown = append(r.shown, p)
		case filterArchived:
			if p.Status == domain.NodeArchived {
				r.shown = append(r.shown, p)
			}
		default: // filterActivePaused
			if p.Status == domain.NodeActive || p.Status == domain.NodePaused {
				r.shown = append(r.shown, p)
			}
		}
	}
	r.cur = r.cur.Clamp(len(r.shown))
}

// isProjectEvent reports whether the SSE event type should trigger a reload.
func isProjectEvent(t string) bool {
	return t == string(domain.EventNodeCreated) ||
		t == string(domain.EventNodeUpdated) ||
		t == string(domain.EventNodeDeleted)
}
