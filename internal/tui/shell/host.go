package shell

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// RouteHost runs a single Route chrome-less (footer only), for the standalone
// `flow <screen>` launch mode. Drill-down (PushRouteMsg) is ignored here — a
// standalone host shows one leaf screen.
type RouteHost struct {
	route         Route
	pal           theme.Palette
	width, height int
}

// NewRouteHost wraps route as a standalone program model.
func NewRouteHost(route Route, pal theme.Palette) RouteHost {
	return RouteHost{route: route, pal: pal}
}

func (h RouteHost) Init() tea.Cmd { return h.route.Init() }

func (h RouteHost) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = msg.Width, msg.Height
		var cmd tea.Cmd
		h.route, cmd = h.route.Update(msg)
		return h, cmd
	case tea.KeyPressMsg:
		if msg.Text == "q" || msg.Code == tea.KeyEsc || (msg.Code == 'c' && msg.Mod == tea.ModCtrl) {
			return h, tea.Quit
		}
	}
	var cmd tea.Cmd
	h.route, cmd = h.route.Update(msg)
	return h, cmd
}

func (h RouteHost) View() tea.View {
	contentH := h.height - 1
	if contentH < 0 {
		contentH = 0
	}
	body := h.route.View(Frame{Width: h.width, Height: contentH, Pal: h.pal})
	footer := keyhint.Render(h.route.KeyHints(), h.pal)
	v := tea.NewView(body + "\n" + footer)
	v.AltScreen = true
	return v
}
