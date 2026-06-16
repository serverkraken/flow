package shell

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// AboutRoute is a static drill-down leaf used to demonstrate push/pop in M3b.
type AboutRoute struct{}

// NewAboutRoute builds the About leaf.
func NewAboutRoute() AboutRoute { return AboutRoute{} }

func (AboutRoute) Title() string                       { return "About" }
func (AboutRoute) Init() tea.Cmd                        { return nil }
func (AboutRoute) Update(tea.Msg) (Route, tea.Cmd)      { return AboutRoute{}, nil }

func (AboutRoute) View(f Frame) string {
	return "\n  " + theme.Strong("flow sidekick-shell", f.Pal) +
		"\n  " + theme.Body("Eine Programm-Shell für alle Screens.", f.Pal) +
		"\n\n  " + theme.Dim("esc -> zurück", f.Pal) + "\n"
}

func (AboutRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{{Key: "esc", Desc: "zurück"}, {Key: "tab", Desc: "Tab"}}
}
