package cli

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/cheatsheet"
	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/spf13/cobra"
)

// CheatsheetDeps is the dependency bundle for the standalone
// `flow cheatsheet` subcommand. Both fields are required — they mirror
// what the sidekick-hosted Cheatsheet factory consumes, so the standalone
// program produces a render identical to the sidekick tab.
type CheatsheetDeps struct {
	Reader   ports.CheatsheetReader
	Renderer ports.MarkdownRenderer
}

// NewCheatsheetCmd constructs the `flow cheatsheet` cobra subcommand —
// a full-screen TUI rendering of the user's tmux cheatsheet through
// flow's own Markdown pipeline (theme.Palette → @tn_* via tmux user
// options, OSC-8 hyperlinks preserved). Replaces the legacy
// `glow -p ~/.tmux/cheatsheet.md` path so theme switches in .tmux.conf
// propagate uniformly. q / Ctrl+C exits.
func NewCheatsheetCmd(deps CheatsheetDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "cheatsheet",
		Short:        "Render the tmux cheatsheet (full-screen TUI)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			tk.Init()
			pal := tk.Load()
			m := cheatsheet.New(pal, deps.Reader, deps.Renderer)
			prog := tea.NewProgram(m, tea.WithContext(cmd.Context()))
			_, err := prog.Run()
			return err
		},
	}
}
