package shell

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// HomeRoute is the placeholder Home screen (M3b). Enter drills into AboutRoute
// to exercise the nav-stack. M3c replaces this with a live dashboard.
type HomeRoute struct{ user string }

// NewHomeRoute builds the Home route for user.
func NewHomeRoute(user string) HomeRoute { return HomeRoute{user: user} }

func (h HomeRoute) Title() string { return "Home" }
func (h HomeRoute) Init() tea.Cmd { return nil }

func (h HomeRoute) Update(msg tea.Msg) (Route, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.Code == tea.KeyEnter {
		return h, func() tea.Msg { return PushRouteMsg{Route: NewAboutRoute()} }
	}
	return h, nil
}

func (h HomeRoute) View(f Frame) string {
	greeting := theme.Heading(fmt.Sprintf("Willkommen, %s", h.user), f.Pal)
	hint := theme.Body("Das Dashboard kommt in M3c.", f.Pal)
	drill := theme.Dim("Enter -> Details (Drill-down-Demo)", f.Pal)
	return fmt.Sprintf("\n  %s\n\n  %s\n  %s\n", greeting, hint, drill)
}

func (h HomeRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "enter", Desc: "Details"},
		{Key: "tab", Desc: "Tab"},
		{Key: ":", Desc: "Palette"},
		{Key: "?", Desc: "Hilfe"},
	}
}
