// Package docs wraps the legacy tui.DocsModel as a shell.Route so the compendium
// screen can live as a tab in `flow ui`. It is a thin adapter — DocsModel is
// unchanged (the Markdown-viewer upgrade is M3d). DocsModel renders its own
// header/footer inside the shell content area (a known M3c3 cosmetic).
package docs

import (
	"os/exec"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// Editor is the editor adapter DocsModel needs (editor.New() satisfies it).
type Editor interface {
	Command(initial []byte) (*exec.Cmd, func() ([]byte, error), func(), error)
}

// Opener opens a URL in the OS browser (opener.New() satisfies it).
type Opener interface {
	Open(url string) error
}

// Route hosts a DocsModel under the shell Route contract.
type Route struct {
	m   tui.DocsModel
	pal theme.Palette
}

// NewRoute builds the Docs route. ed/op may be nil in tests (the $EDITOR/open
// paths are never hit there).
func NewRoute(client *apiclient.Client, ed Editor, op Opener, pal theme.Palette, user string) *Route {
	return &Route{m: tui.NewDocs(client, ed, op, user), pal: pal}
}

func (r *Route) Title() string { return "Docs" }
func (r *Route) Init() tea.Cmd { return r.m.Init() }

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	nm, cmd := r.m.Update(msg)
	r.m = nm.(tui.DocsModel)
	return r, cmd
}

func (r *Route) View(shell.Frame) string { return r.m.View().Content }

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "j/k", Desc: "wählen"},
		{Key: "enter", Desc: "öffnen"},
		{Key: "n", Desc: "neu"},
		{Key: "e", Desc: "edit"},
		{Key: "/", Desc: "suchen"},
		{Key: "f", Desc: "Filter"},
	}
}
