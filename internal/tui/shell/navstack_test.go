package shell_test

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
)

func TestNavStack_pushPopFloor(t *testing.T) {
	ns := shell.NewNavStack(stubRoute{title: "Home"})
	if ns.Top().Title() != "Home" || ns.Len() != 1 {
		t.Fatal("fresh stack")
	}
	ns.Pop() // floor: no-op at depth 1
	if ns.Len() != 1 {
		t.Fatal("pop floor")
	}
	ns.Push(stubRoute{title: "Detail"})
	if ns.Top().Title() != "Detail" || ns.Len() != 2 {
		t.Fatal("push")
	}
	ns.Pop()
	if ns.Top().Title() != "Home" || ns.Len() != 1 {
		t.Fatal("pop")
	}
}

func TestNavStack_crumbs(t *testing.T) {
	ns := shell.NewNavStack(stubRoute{title: "Docs"})
	ns.Push(stubRoute{title: "Note"})
	if got := ns.Crumbs(); !reflect.DeepEqual(got, []string{"Docs", "Note"}) {
		t.Fatalf("crumbs = %v", got)
	}
}

func TestNavStack_updateTopReplaces(t *testing.T) {
	ns := shell.NewNavStack(stubRoute{title: "Home"})
	_ = ns.UpdateTop(tea.KeyPressMsg{Code: tea.KeyDown}) // returns tea.Cmd; must not panic
	if ns.Top().Title() != "Home" {
		t.Fatal("update top kept identity")
	}
}
