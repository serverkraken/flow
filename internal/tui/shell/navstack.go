package shell

import tea "charm.land/bubbletea/v2"

// NavStack is one tab's LIFO stack of Routes. Index 0 is the permanent root:
// Pop() is a no-op at depth 1.
type NavStack struct {
	stack []Route
}

// NewNavStack returns a stack whose only (permanent) entry is root.
func NewNavStack(root Route) *NavStack { return &NavStack{stack: []Route{root}} }

// Top returns the visible Route.
func (n *NavStack) Top() Route { return n.stack[len(n.stack)-1] }

// Len returns the stack depth.
func (n *NavStack) Len() int { return len(n.stack) }

// Push adds r as the new top (drill-down).
func (n *NavStack) Push(r Route) { n.stack = append(n.stack, r) }

// Pop removes the top Route; no-op at depth 1.
func (n *NavStack) Pop() {
	if len(n.stack) > 1 {
		n.stack = n.stack[:len(n.stack)-1]
	}
}

// ReplaceTop swaps the top Route without changing depth.
func (n *NavStack) ReplaceTop(r Route) { n.stack[len(n.stack)-1] = r }

// UpdateTop forwards msg to the top Route, stores the returned Route, and
// returns its command.
func (n *NavStack) UpdateTop(msg tea.Msg) tea.Cmd {
	next, cmd := n.Top().Update(msg)
	n.ReplaceTop(next)
	return cmd
}

// Crumbs returns the titles from root to top, for the breadcrumb.
func (n *NavStack) Crumbs() []string {
	out := make([]string, len(n.stack))
	for i, r := range n.stack {
		out[i] = r.Title()
	}
	return out
}
