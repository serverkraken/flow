// Package wtnav routes a sibling-navigation key (w/t/d/e) to a SwitchRouteMsg
// carrying a freshly-built target Route. The factory map is built once in the
// worktime hub (which imports the leaf packages) and injected into every route,
// so leaves never import each other. wtnav imports only shell, breaking the
// cycle lateral navigation would otherwise create.
package wtnav

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/tabstrip"
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

// Sub-tab indices into SubTabs; each Worktime route declares which one it is.
const (
	IdxHeute = iota
	IdxWoche
	IdxStats
	IdxFrei
	IdxExport
)

// SubTab is one Worktime sub-tab: the strip label and its accelerator key
// ("" for Heute, the nav-stack root reached by popping back).
type SubTab struct {
	Label string
	Key   string
}

// SubTabs is the single source of truth for Worktime sub-tab order, labels, and
// accelerator keys. Positions match the Idx* constants.
var SubTabs = []SubTab{
	{Label: "Heute", Key: ""},
	{Label: "Woche", Key: "w"},
	{Label: "Stats", Key: "t"},
	{Label: "Frei", Key: "d"},
	{Label: "Export", Key: "e"},
}

// Strip renders the Worktime sub-tab strip with active highlighted, reusing the
// shell's top-tab component so it looks identical one level down.
func Strip(active, width int, pal theme.Palette) string {
	labels := make([]string, len(SubTabs))
	for i, t := range SubTabs {
		labels[i] = t.Label
	}
	return tabstrip.Render(labels, active, width, pal)
}

// Lateral maps ←/→ to a sub-tab navigation command relative to current. ← / →
// step one tab (clamped, no wrap). Stepping to Heute from a sibling pops back to
// the root (Heute's live clock resumes via the shell's pop-re-Init); stepping to
// a sibling emits a SwitchRouteMsg through the registry. Returns nil for a
// non-arrow key or a no-op step, so the caller keeps handling the key.
func Lateral(reg Registry, current int, k tea.KeyPressMsg) tea.Cmd {
	var target int
	switch k.Code {
	case tea.KeyLeft:
		target = current - 1
	case tea.KeyRight:
		target = current + 1
	default:
		return nil
	}
	if target < 0 || target >= len(SubTabs) || target == current {
		return nil
	}
	if target == IdxHeute {
		return func() tea.Msg { return shell.PopRouteMsg{} }
	}
	return reg.Nav(SubTabs[target].Key)
}
