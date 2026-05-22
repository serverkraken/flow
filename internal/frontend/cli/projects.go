package cli

// `flow projects` — Standalone-Variante des Projects-Screens für den
// tmux-display-popup-Aufruf. Ersetzt das legacy `project-switcher.sh`
// (32 Zeilen sh + fd + fzf); der Plan ist in CLAUDE-tmux-migration-
// plan.md dokumentiert.

import (
	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// ProjectsDeps is the dependency bundle for the standalone
// `flow projects` subcommand.
type ProjectsDeps struct {
	Screen func(tk.Palette) tea.Model
}

// NewProjectsCmd konstruiert das `flow projects` Cobra-Subcommand für
// den tmux-Plugin-Aufruf:
//
//	bind f display-popup -E -T " Projekt " -w 80% -h 80% \
//	    "zsh -c 'flow projects'"
//
// Nach erfolgreichem `switch-client` quittet das Programm (tmux hat
// den Client umgehängt); bei Fehler bleibt das Programm offen und
// surfaced den Danger-Toast.
func NewProjectsCmd(deps ProjectsDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "projects",
		Short:        "Run the project switcher (full-screen TUI for tmux popup)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			tk.Init()
			pal := tk.Load()
			m := deps.Screen(pal)
			prog := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(cmd.Context()))
			_, err := prog.Run()
			return err
		},
	}
}
