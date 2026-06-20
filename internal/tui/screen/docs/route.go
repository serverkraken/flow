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
	return &Route{m: tui.NewDocs(client, ed, op, pal, user), pal: pal}
}

func (r *Route) Title() string { return "Docs" }
func (r *Route) Init() tea.Cmd { return r.m.Init() }

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	nm, cmd := r.m.Update(msg)
	r.m = nm.(tui.DocsModel)
	return r, cmd
}

// View bridges the host frame size into the wrapped DocsModel (so the fullscreen
// markdown viewer overlay is sized from f.Width/f.Height, never a stored
// WindowSizeMsg) and renders the model's content.
func (r *Route) View(f shell.Frame) string {
	r.m = r.m.SetViewport(f.Width, f.Height)
	return r.m.View().Content
}

// FullScreen reports fullscreen while the docs screen is reading a document, so
// the shell suppresses its chrome and hands the viewer the whole terminal.
// Implements shell.FullScreener.
func (r *Route) FullScreen() bool { return r.m.InViewMode() }

// CapturesInput delegates to the wrapped DocsModel so the shell forwards every
// key to the docs screen while it is in a text-entry / sub-mode (e.g. the New
// Document form's Tab field-nav or Esc cancel). Without this the shell treats
// Tab as tab-switch and the form keys never arrive. Implements
// shell.InputCapturer.
func (r *Route) CapturesInput() bool { return r.m.CapturesInput() }

// CapturesText implements shell.TextCapturer.
func (r *Route) CapturesText() bool { return r.m.CapturesText() }

// Back implements shell.Backer. It must NOT mutate the receiver: ResolveBack
// calls Back() once to probe `handled` and the shell calls it again to apply,
// so Back() has to be referentially transparent — it returns a new route
// carrying the resolved model rather than mutating r.m in place (a pointer-
// receiver `r.m = nm` would double-pop the docs viewStack, skipping a level).
func (r *Route) Back() (shell.Route, tea.Cmd, bool) {
	nm, cmd, ok := r.m.Back()
	nr := *r // shallow copy: pal is a value; m is replaced below
	nr.m = nm
	return &nr, cmd, ok
}

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "j/k", Desc: "wählen"},
		{Key: "enter", Desc: "öffnen"},
		{Key: "n", Desc: "neu"},
		{Key: "e", Desc: "edit"},
		{Key: "p", Desc: "Projekt"},
		{Key: "/", Desc: "suchen"},
		{Key: "f", Desc: "Filter"},
	}
}
