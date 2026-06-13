package cli

// `flow projects` — two modes in one cobra command tree:
//
//  1. `flow projects` (no subcommand) — Standalone SourceDirs TUI für den
//     tmux-display-popup-Aufruf. Ersetzt das legacy `project-switcher.sh`.
//     Load-bearing: tmux binding `bind f display-popup -E 'zsh -c "flow projects"'`
//     muss erhalten bleiben.
//
//  2. `flow projects list|create|rename|archive` — Worktime-Project CRUD
//     gegen die sqliteclient.Store (wired in cmd/flow/main.go, Task 13).

import (
	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
)

// ProjectsDeps is the dependency bundle for the `flow projects` command tree.
// Screen drives the standalone TUI (tmux popup use case).
// CRUD and ProjectStore are optional: when nil the CRUD subcommands are not
// registered and `flow projects` behaves as before Task 13.
type ProjectsDeps struct {
	Screen       func(tk.Palette) tea.Model
	CRUD         *ProjectsCRUDDeps  // nil → CRUD subcommands not registered
	ProjectStore ports.ProjectStore // needed by rename/archive for slug resolution
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
//
// When deps.CRUD is non-nil the list/create/rename/archive subcommands are
// also registered; `flow projects` (no subcommand) still launches the TUI.
func NewProjectsCmd(deps ProjectsDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "projects",
		Short:        "Run the project switcher (full-screen TUI for tmux popup)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			tk.Init()
			pal := tk.Load()
			m := deps.Screen(pal)
			prog := tea.NewProgram(m, tea.WithContext(cmd.Context()))
			_, err := prog.Run()
			return err
		},
	}

	if deps.CRUD != nil {
		cmd.AddCommand(
			newProjectsListCmd(*deps.CRUD),
			newProjectsCreateCmd(*deps.CRUD),
			newProjectsRenameCmd(*deps.CRUD, deps.ProjectStore),
			newProjectsArchiveCmd(*deps.CRUD, deps.ProjectStore),
		)
	}

	return cmd
}
