package nodetree

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
	"github.com/serverkraken/flow/internal/tui/ui/grammar"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/listnav"
	"github.com/serverkraken/flow/internal/tui/ui/toast"
)

type dialogKind int

const (
	dialogNone   dialogKind = iota
	dialogDelete            // waiting for confirm.ResultMsg
	dialogMove              // waiting for fuzzylist selection
)

type loadedMsg struct {
	nodes []domain.Node
	err   error
}

type reloadMsg struct{}

// Route is the "Knoten" tab root. Implements shell.Route + shell.InputCapturer.
type Route struct {
	api  TreeAPI
	pal  theme.Palette
	user string

	all       []domain.Node
	rows      []Row
	cur       listnav.Cursor
	kind      domain.NodeKind // "" = all
	query     string
	filtering bool

	loaded bool
	err    error
	toast  toast.Model

	dialog  dialogKind
	confirm confirm.Model
	delID   string
	move    moveState

	detailFor func(domain.Node) shell.Route
	formFor   func(*domain.Node) shell.Route // nil ptr → create; non-nil → edit
}

// NewRoute constructs the Knoten tree route.
func NewRoute(api TreeAPI, pal theme.Palette, user string) *Route {
	return &Route{api: api, pal: pal, user: user, cur: listnav.New()}
}

// SetDetailFactory registers the factory that produces a detail Route for a given Node.
func (r *Route) SetDetailFactory(f func(domain.Node) shell.Route) { r.detailFor = f }

// SetFormFactory registers the factory that produces an edit/create Route for a Node pointer.
// nil pointer → create mode; non-nil pointer → edit mode.
func (r *Route) SetFormFactory(f func(*domain.Node) shell.Route) { r.formFor = f }

// Title implements shell.Route.
func (r *Route) Title() string { return "Knoten" }

// Init implements shell.Route — triggers the first load.
func (r *Route) Init() tea.Cmd { return r.loadCmd() }

func (r *Route) loadCmd() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ns, err := api.ListNodes(ctx)
		return loadedMsg{nodes: ns, err: err}
	}
}

func (r *Route) rebuild() {
	rows := BuildTree(r.all)
	rows = FilterKind(rows, r.kind)
	rows = FuzzyFilter(rows, r.query)
	r.rows = rows
	r.cur = r.cur.Clamp(len(r.rows))
}

func (r *Route) selected() (domain.Node, bool) {
	i := r.cur.Index()
	if i >= 0 && i < len(r.rows) {
		return r.rows[i].Node, true
	}
	return domain.Node{}, false
}

// Update implements shell.Route.
func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded = true
		r.all, r.err = m.nodes, m.err
		r.rebuild()
		return r, nil

	case reloadMsg:
		return r, r.loadCmd()

	case shell.EventMsg:
		if isNodeEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil

	case toast.DismissedMsg:
		r.toast, _ = r.toast.Update(m)
		return r, nil

	case confirm.ResultMsg:
		open := r.dialog == dialogDelete
		r.dialog = dialogNone
		if open && m.Confirmed && r.delID != "" {
			id := r.delID
			api := r.api
			return r, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := api.DeleteNode(ctx, id); err != nil {
					return deleteErrMsg{err}
				}
				return reloadMsg{}
			}
		}
		return r, nil

	case deleteErrMsg:
		r.toast = toast.NewDanger(deleteErrText(m.err), r.pal)
		return r, r.toast.Init()

	case moveDoneMsg:
		if m.err != nil {
			r.toast = toast.NewDanger("Verschieben: "+m.err.Error(), r.pal)
			return r, r.toast.Init()
		}
		return r, r.loadCmd()

	case tea.KeyPressMsg:
		return r.handleKey(m)
	}

	return r, nil
}

func (r *Route) handleKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	if r.dialog != dialogNone {
		return r.handleDialogKey(k)
	}
	if r.filtering {
		return r.handleFilterKey(k)
	}

	// Arrow / Home / End / PgUp-Dn via listnav.
	if cur, ok := r.cur.Handle(k, len(r.rows), 5); ok {
		r.cur = cur
		return r, nil
	}

	switch {
	case k.Text == "j":
		r.cur = r.cur.Set(r.cur.Index()+1, len(r.rows))
		return r, nil

	case k.Text == "k":
		r.cur = r.cur.Set(r.cur.Index()-1, len(r.rows))
		return r, nil

	case grammar.WeekNext.Matches(k): // ]
		r.kind = nextKind(r.kind, +1)
		r.rebuild()
		return r, nil

	case grammar.WeekPrev.Matches(k): // [
		r.kind = nextKind(r.kind, -1)
		r.rebuild()
		return r, nil

	case grammar.Search.Matches(k): // /
		r.filtering = true
		return r, nil

	case grammar.New.Matches(k): // n
		if r.formFor != nil {
			return r, push(r.formFor(nil))
		}
		return r, nil

	case grammar.Edit.Matches(k): // e
		if n, ok := r.selected(); ok && r.formFor != nil {
			cp := n
			return r, push(r.formFor(&cp))
		}
		return r, nil

	case k.Text == "m":
		return r.openMove()

	case k.Text == "D":
		return r.openDelete()

	case grammar.Open.Matches(k): // enter
		if n, ok := r.selected(); ok && r.detailFor != nil {
			return r, push(r.detailFor(n))
		}
		return r, nil
	}

	return r, nil
}

func (r *Route) handleFilterKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch k.Code {
	case tea.KeyEsc:
		r.filtering = false
		r.query = ""
		r.rebuild()
		return r, nil

	case tea.KeyEnter:
		r.filtering = false
		if n, ok := r.selected(); ok && r.detailFor != nil {
			return r, push(r.detailFor(n))
		}
		return r, nil

	case tea.KeyBackspace:
		if rn := []rune(r.query); len(rn) > 0 {
			r.query = string(rn[:len(rn)-1])
		}
		r.rebuild()
		return r, nil
	}

	// Arrow keys still move the cursor while filtering.
	if cur, ok := r.cur.Handle(k, len(r.rows), 5); ok {
		r.cur = cur
		return r, nil
	}

	// Every other printable rune goes into the query.
	if k.Text != "" {
		r.query += k.Text
		r.rebuild()
	}

	return r, nil
}

// CapturesInput implements shell.InputCapturer — own the keyboard while a
// dialog is open or while typing a fuzzy filter.
func (r *Route) CapturesInput() bool { return r.dialog != dialogNone || r.filtering }

// View implements shell.Route.
func (r *Route) View(f shell.Frame) string { return renderView(r, f) }

// KeyHints implements shell.Route.
func (r *Route) KeyHints() []keyhint.Hint {
	if r.dialog == dialogDelete {
		return []keyhint.Hint{{Key: "y", Desc: "löschen"}, {Key: "n", Desc: "abbrechen"}}
	}
	if r.dialog == dialogMove {
		return []keyhint.Hint{
			{Key: "↑/↓", Desc: "Ziel"},
			{Key: "enter", Desc: "verschieben"},
			{Key: "esc", Desc: "abbrechen"},
		}
	}
	if r.filtering {
		return []keyhint.Hint{
			{Key: "tippen", Desc: "filtern"},
			{Key: "enter", Desc: "öffnen"},
			{Key: "esc", Desc: "abbrechen"},
		}
	}
	return []keyhint.Hint{
		grammar.Open.Hint(),
		grammar.New.Hint(),
		grammar.Edit.Hint(),
		{Key: "m", Desc: "verschieben"},
		{Key: "D", Desc: "löschen"},
		grammar.Search.Hint(),
		{Key: "[ ]", Desc: "Filter"},
		grammar.MoveUp.Hint(),
	}
}

// push emits a PushRouteMsg for the given child route.
func push(child shell.Route) tea.Cmd {
	return func() tea.Msg { return shell.PushRouteMsg{Route: child} }
}

func isNodeEvent(t string) bool {
	switch domain.EventType(t) {
	case domain.EventNodeCreated, domain.EventNodeUpdated, domain.EventNodeMoved, domain.EventNodeDeleted:
		return true
	}
	return false
}

// kindCycle is the filter cycle: all → engagement → vorhaben → repo → all.
var kindCycle = []domain.NodeKind{"", domain.KindEngagement, domain.KindVorhaben, domain.KindRepo}

func nextKind(cur domain.NodeKind, dir int) domain.NodeKind {
	i := 0
	for j, k := range kindCycle {
		if k == cur {
			i = j
			break
		}
	}
	return kindCycle[(i+dir+len(kindCycle))%len(kindCycle)]
}

// kindFilterLabel returns the human-readable label for the current kind filter.
func kindFilterLabel(k domain.NodeKind) string {
	switch k {
	case domain.KindEngagement:
		return "Engagements"
	case domain.KindVorhaben:
		return "Vorhaben"
	case domain.KindRepo:
		return "Repos"
	default:
		return "alle"
	}
}
