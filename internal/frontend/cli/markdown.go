package cli

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	tk "github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/view"
	"github.com/spf13/cobra"
)

// NewMarkdownCmd constructs the `flow markdown` cobra subtree. The
// `view <file>` verb opens an arbitrary Markdown file in the same
// in-process viewer kompendium browse uses for `v` — glamour render
// pipeline, OSC 8 hyperlinks, search via `/`, code-snippet copy via
// `c`. Wikilink resolution is intentionally disabled (resolver = nil)
// because a free-floating Markdown file has no notebook context;
// `[[wikilinks]]` render as broken markers.
func NewMarkdownCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "markdown",
		Short: "Markdown utilities",
	}
	cmd.AddCommand(newMarkdownViewCmd())
	return cmd
}

func newMarkdownViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "view <file>",
		Short:        "Open a Markdown file in the full-screen TUI viewer",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			path := args[0]
			source, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			tk.Init()
			_ = tk.Load() // initialise the truecolor renderer; viewer reads its own theme.
			m := view.New(filepath.Base(path), string(source), nil, nil, nil)
			prog := tea.NewProgram(markdownViewerProgram{inner: m}, tea.WithAltScreen())
			_, err = prog.Run()
			return err
		},
	}
}

// markdownViewerProgram adapts the kompendium viewer (which signals
// dismissal via view.ExitMsg, expecting a hosting model to handle it)
// into a standalone tea.Model that quits on ExitMsg. The viewer's
// concrete-typed Update is forwarded through unchanged.
type markdownViewerProgram struct {
	inner view.Model
}

func (p markdownViewerProgram) Init() tea.Cmd { return p.inner.Init() }

func (p markdownViewerProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(view.ExitMsg); ok {
		return p, tea.Quit
	}
	next, cmd := p.inner.Update(msg)
	p.inner = next
	return p, cmd
}

func (p markdownViewerProgram) View() string { return p.inner.View() }
