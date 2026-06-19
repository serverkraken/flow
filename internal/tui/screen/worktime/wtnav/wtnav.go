// Package wtnav routes a sibling-navigation key (w/t/d/e) to a SwitchRouteMsg
// carrying a freshly-built target Route. The factory map is built once in the
// worktime hub (which imports the leaf packages) and injected into every route,
// so leaves never import each other. wtnav imports only shell, breaking the
// cycle lateral navigation would otherwise create.
package wtnav

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
)

// Registry maps a sibling key ("w","t","d","e") to a lazy Route factory.
type Registry map[string]func() shell.Route

// Nav returns a cmd emitting shell.SwitchRouteMsg for key, or nil when key has
// no registered factory (so pressing an unmapped key is a no-op).
func (r Registry) Nav(key string) tea.Cmd {
	factory, ok := r[key]
	if !ok || factory == nil {
		return nil
	}
	return func() tea.Msg { return shell.SwitchRouteMsg{Route: factory()} }
}
