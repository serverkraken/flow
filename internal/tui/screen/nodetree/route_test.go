package nodetree

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
)

// fakeTreeAPI is a test-only implementation of TreeAPI.
type fakeTreeAPI struct {
	nodes   []domain.Node
	deleted []string
	moved   []struct{ id, parent string }
	delErr  error
}

func (f *fakeTreeAPI) ListNodes(context.Context) ([]domain.Node, error) { return f.nodes, nil }
func (f *fakeTreeAPI) DeleteNode(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return f.delErr
}
func (f *fakeTreeAPI) MoveNode(_ context.Context, id string, parentID *string) (domain.Node, error) {
	p := ""
	if parentID != nil {
		p = *parentID
	}
	f.moved = append(f.moved, struct{ id, parent string }{id, p})
	return domain.Node{}, nil
}

// clientEvent builds a ClientEvent for SSE reload tests.
func clientEvent(t string) apiclient.ClientEvent { return apiclient.ClientEvent{Type: t} }

// key builds a KeyPressMsg for a printable rune.
func key(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// loaded drives the Route into a loaded state with three test nodes.
func loaded(r *Route) {
	r.Update(loadedMsg{nodes: []domain.Node{
		{ID: "e1", Kind: domain.KindEngagement, Name: "Privat"},
		{ID: "r1", ParentID: sp("e1"), Kind: domain.KindRepo, Name: "flow"},
		{ID: "e2", Kind: domain.KindEngagement, Name: "RTL Extern"},
	}})
}

func TestRoute_LoadBuildsTree(t *testing.T) {
	t.Parallel()
	r := NewRoute(&fakeTreeAPI{}, theme.Default, "u")
	loaded(r)
	if len(r.rows) != 3 || r.rows[0].Node.ID != "e1" || r.rows[1].Node.ID != "r1" {
		t.Fatalf("tree not built pre-order: %+v", r.rows)
	}
}

func TestRoute_CursorJK(t *testing.T) {
	t.Parallel()
	r := NewRoute(&fakeTreeAPI{}, theme.Default, "u")
	loaded(r)
	r.Update(key('j'))
	if r.cur.Index() != 1 {
		t.Fatalf("j → cursor 1, got %d", r.cur.Index())
	}
	r.Update(key('k'))
	if r.cur.Index() != 0 {
		t.Fatalf("k → cursor 0, got %d", r.cur.Index())
	}
}

func TestRoute_KindFilterCycle(t *testing.T) {
	t.Parallel()
	r := NewRoute(&fakeTreeAPI{}, theme.Default, "u")
	loaded(r)
	r.Update(key(']')) // alle → Engagements
	if r.kind != domain.KindEngagement || len(r.rows) != 2 {
		t.Fatalf("] → Engagements (2 rows), got kind=%q rows=%d", r.kind, len(r.rows))
	}
}

func TestRoute_FuzzyFilterMode(t *testing.T) {
	t.Parallel()
	r := NewRoute(&fakeTreeAPI{}, theme.Default, "u")
	loaded(r)
	r.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !r.filtering || !r.CapturesInput() {
		t.Fatal("/ must enter filtering mode + capture input")
	}
	for _, c := range "flow" {
		r.Update(key(c))
	}
	if r.query != "flow" {
		t.Fatalf("query = %q, want flow", r.query)
	}
	ids := map[string]bool{}
	for _, row := range r.rows {
		ids[row.Node.ID] = true
	}
	if !ids["r1"] || !ids["e1"] || ids["e2"] {
		t.Fatalf("fuzzy rows wrong: %v", ids)
	}
	r.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if r.filtering || r.query != "" {
		t.Fatal("esc must clear + exit filter")
	}
}

func TestRoute_DeleteConfirmCallsAPI(t *testing.T) {
	t.Parallel()
	f := &fakeTreeAPI{}
	r := NewRoute(f, theme.Default, "u")
	loaded(r)
	r.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	if r.dialog != dialogDelete {
		t.Fatal("D must open delete confirm")
	}
	_, cmd := r.Update(confirm.ResultMsg{Confirmed: true})
	if cmd == nil {
		t.Fatal("confirmed delete must return a cmd")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("delete cmd produced no msg")
	}
	if len(f.deleted) != 1 || f.deleted[0] != "e1" {
		t.Fatalf("DeleteNode not called for e1: %v", f.deleted)
	}
}

func TestRoute_MoveDialogCallsAPI(t *testing.T) {
	t.Parallel()
	f := &fakeTreeAPI{}
	r := NewRoute(f, theme.Default, "u")
	loaded(r)
	r.Update(key('j')) // cursor → r1 (repo)
	r.Update(key('m'))
	if r.dialog != dialogMove {
		t.Fatal("m must open move dialog")
	}
	// candidates for repo r1 = engagements e1,e2 (sorted: Privat, RTL Extern)
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("move enter must return a cmd")
	}
	cmd()
	if len(f.moved) != 1 || f.moved[0].id != "r1" {
		t.Fatalf("MoveNode not called for r1: %v", f.moved)
	}
}

func TestRoute_EnterPushesDetail(t *testing.T) {
	t.Parallel()
	r := NewRoute(&fakeTreeAPI{}, theme.Default, "u")
	var got domain.Node
	r.SetDetailFactory(func(n domain.Node) shell.Route { got = n; return nil })
	loaded(r)
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must push detail")
	}
	if _, ok := cmd().(shell.PushRouteMsg); !ok {
		t.Fatal("enter cmd must emit PushRouteMsg")
	}
	if got.ID != "e1" {
		t.Fatalf("detail factory got %q, want e1", got.ID)
	}
}

func TestRoute_SSEReload(t *testing.T) {
	t.Parallel()
	r := NewRoute(&fakeTreeAPI{}, theme.Default, "u")
	_, cmd := r.Update(shell.EventMsg{Ev: clientEvent("node.moved")})
	if cmd == nil {
		t.Fatal("node.moved must trigger reload")
	}
}
