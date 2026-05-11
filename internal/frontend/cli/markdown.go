package cli

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/markdown"
	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
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
	var rawMode bool
	var rawWidth int
	cmd := &cobra.Command{
		Use:          "view <file>",
		Short:        "Open a Markdown file in the full-screen TUI viewer",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			source, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			tk.Init()
			_ = tk.Load() // initialise the truecolor renderer.

			if rawMode {
				// Bypass the TUI: render the markdown to ANSI text and
				// write straight to stdout. Useful for diagnostics
				// (`flow markdown view --raw -w 100 f.md | xxd`) and for
				// piping into pagers / less. Force truecolor so SGRs
				// emit even though stdout is a pipe (lipgloss otherwise
				// detects Ascii and strips every colour code).
				lipgloss.SetColorProfile(termenv.TrueColor)
				out, rerr := markdown.Render(string(source), rawWidth)
				if rerr != nil {
					return fmt.Errorf("render: %w", rerr)
				}
				_, werr := fmt.Fprint(os.Stdout, out)
				return werr
			}

			render := func(src string, w int) string {
				out, _ := markdown.Render(src, w)
				return out
			}
			m := markdown_overlay.New(render,
				markdown_overlay.WithTitle(filepath.Base(path)),
				markdown_overlay.WithSource(string(source)),
				markdown_overlay.WithSearch(),
				markdown_overlay.WithCodeCopy(),
			)
			prog := tea.NewProgram(markdownViewerProgram{inner: m},
				tea.WithAltScreen(), tea.WithContext(cmd.Context()))
			_, err = prog.Run()
			return err
		},
	}
	cmd.Flags().BoolVar(&rawMode, "raw", false,
		"Bypass the TUI and write the rendered ANSI text to stdout (for diagnostics + pagers).")
	cmd.Flags().IntVarP(&rawWidth, "width", "w", 100,
		"Render width in columns when --raw is set (no terminal-size detection in raw mode).")
	return cmd
}

// markdownViewerProgram adapts the markdown overlay (which signals
// dismissal via markdown_overlay.ExitMsg, expecting a hosting model
// to handle it) into a standalone tea.Model that quits on ExitMsg.
// The overlay's concrete-typed Update is forwarded through unchanged.
type markdownViewerProgram struct {
	inner markdown_overlay.Model
}

func (p markdownViewerProgram) Init() tea.Cmd { return p.inner.Init() }

func (p markdownViewerProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(markdown_overlay.ExitMsg); ok {
		return p, tea.Quit
	}
	next, cmd := p.inner.Update(msg)
	p.inner = next
	return p, cmd
}

func (p markdownViewerProgram) View() string { return p.inner.View() }
