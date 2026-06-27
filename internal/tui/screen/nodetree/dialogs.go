package nodetree

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
)

// ---- delete -----------------------------------------------------------------

type deleteErrMsg struct{ err error }

func (r *Route) openDelete() (shell.Route, tea.Cmd) {
	n, ok := r.selected()
	if !ok {
		return r, nil
	}
	r.delID = n.ID
	r.confirm = confirm.NewDanger("Knoten löschen?", n.Name, r.pal)
	r.dialog = dialogDelete
	return r, nil
}

// deleteErrText surfaces ports.ErrNodeHasChildren (RESTRICT) as a clear hint.
// Called only when err != nil (deleteErrMsg is only produced on error), so the
// final fallback is defensive only.
func deleteErrText(err error) string {
	if err != nil && strings.Contains(err.Error(), "children") {
		return "Knoten hat Unterknoten — erst leeren oder umhängen"
	}
	if err != nil {
		return "Löschen fehlgeschlagen: " + err.Error()
	}
	return "Löschen fehlgeschlagen" // defensive: err is always non-nil at call site
}

// ---- move (reparent) --------------------------------------------------------

type moveState struct {
	node  domain.Node
	list  fuzzylist.Model
	cands []domain.Node
}

type moveDoneMsg struct{ err error }

func (r *Route) openMove() (shell.Route, tea.Cmd) {
	n, ok := r.selected()
	if !ok {
		return r, nil
	}
	cands := MoveCandidates(r.all, n)
	r.move = moveState{
		node:  n,
		cands: cands,
		list:  fuzzylist.New(candItems(cands), r.pal),
	}
	r.dialog = dialogMove
	return r, nil
}

func candItems(ns []domain.Node) []fuzzylist.Item {
	out := make([]fuzzylist.Item, 0, len(ns))
	for _, n := range ns {
		out = append(out, fuzzylist.Item{
			ID:    n.ID,
			Label: n.Name + " (" + string(n.Kind) + ")",
		})
	}
	return out
}

func (r *Route) handleDialogKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch r.dialog {
	case dialogDelete:
		m, cmd := r.confirm.Update(k)
		r.confirm = m
		return r, cmd
	case dialogMove:
		return r.handleMoveKey(k)
	}
	return r, nil
}

func (r *Route) handleMoveKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch k.Code {
	case tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil

	case tea.KeyEnter:
		it, _, ok := r.move.list.Selection()
		if !ok {
			r.dialog = dialogNone
			return r, nil
		}
		id, parent, api := r.move.node.ID, it.ID, r.api
		r.dialog = dialogNone
		return r, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := api.MoveNode(ctx, id, &parent); err != nil {
				return moveDoneMsg{err}
			}
			return moveDoneMsg{}
		}

	default:
		r.move.list = r.move.list.Update(k)
		return r, nil
	}
}

func (r *Route) renderMove(f shell.Frame) string {
	pal := f.Pal
	if pal.Bg == "" {
		pal = r.pal
	}
	var b strings.Builder
	b.WriteString("\n  \"" + r.move.node.Name + "\" verschieben unter ...")
	b.WriteString("  " + theme.Dim("tippen: filtern  ↑/↓: Ziel  enter: verschieben  esc", pal))
	b.WriteString("\n\n")
	if len(r.move.cands) == 0 {
		b.WriteString(theme.Dim("  Keine gültigen Ziele für diesen Knotentyp.", pal))
		return b.String()
	}
	b.WriteString(r.move.list.View(f.Width - 4))
	return b.String()
}
