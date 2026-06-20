package fuzzylist_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
)

func items() []fuzzylist.Item {
	return []fuzzylist.Item{{ID: "1", Label: "serverkraken/flow"}, {ID: "2", Label: "backstage"}, {ID: "3", Label: "oraya"}}
}

func TestFilterNarrowsAndSelects(t *testing.T) {
	t.Parallel()
	m := fuzzylist.New(items(), theme.Default)
	m = m.Update(tea.KeyPressMsg{Text: "o"}) // matches flow(o), backstage? no 'o'… 'oraya','flow' have o
	if m.Query() != "o" {
		t.Fatalf("query = %q, want o", m.Query())
	}
	it, isCreate, ok := m.Selection()
	if !ok || isCreate {
		t.Fatalf("expected a real selection, got ok=%v isCreate=%v", ok, isCreate)
	}
	_ = it
}

func TestTypingRoutesToQueryNotNav(t *testing.T) {
	t.Parallel()
	m := fuzzylist.New(items(), theme.Default)
	m = m.Update(tea.KeyPressMsg{Text: "j"}) // 'j' is a typed char, not navigation
	if m.Query() != "j" {
		t.Errorf("query = %q, want j (j must be typed, not navigation)", m.Query())
	}
}

func TestArrowAndCtrlNavigation(t *testing.T) {
	t.Parallel()
	m := fuzzylist.New(items(), theme.Default) // 3 items, cursor 0
	m = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if _, _, ok := m.Selection(); !ok {
		t.Fatal("selection should exist after Down")
	}
	m = m.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}) // ctrl+n = down
	m = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})           // clamp at last
	it, _, _ := m.Selection()
	if it.ID != "3" {
		t.Errorf("cursor should clamp at last item (oraya), got %q", it.ID)
	}
	m = m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}) // ctrl+p = up
	if it, _, _ := m.Selection(); it.ID != "2" {
		t.Errorf("ctrl+p should move up to backstage, got %q", it.ID)
	}
}

func TestInlineCreate(t *testing.T) {
	t.Parallel()
	m := fuzzylist.New(items(), theme.Default).WithCreateHint("neu: %s")
	m = m.Update(tea.KeyPressMsg{Text: "z"}) // no item matches 'z' exactly → create row appears
	m = m.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // move onto the create row (it's last)
	// move cursor to the create row regardless of how many matched
	for i := 0; i < 5; i++ {
		m = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	it, isCreate, ok := m.Selection()
	if !ok || !isCreate {
		t.Fatalf("expected create selection, got it=%+v isCreate=%v ok=%v", it, isCreate, ok)
	}
	if m.Query() != "z" {
		t.Errorf("query for create = %q, want z", m.Query())
	}
}

func TestSetItemsPreservesQuery(t *testing.T) {
	t.Parallel()
	m := fuzzylist.New(nil, theme.Default)
	m = m.Update(tea.KeyPressMsg{Text: "f"})
	m = m.SetItems(items())
	if m.Query() != "f" {
		t.Errorf("SetItems dropped query: %q", m.Query())
	}
	if it, _, ok := m.Selection(); !ok || it.ID != "1" {
		t.Errorf("after SetItems with query 'f', expected flow selected, got %+v ok=%v", it, ok)
	}
}
