package cli

// `flow palette` — Standalone-Variante des Palette-Screens für den
// tmux-display-popup-Aufruf. Ersetzt das legacy `palette.sh` (165
// Zeilen bash + awk + fzf); der Plan ist in CLAUDE-tmux-migration-
// plan.md dokumentiert.
//
// Unterscheidet sich vom Sidekick-Tab nur in der Dispatch-Semantik:
// goto.sh-Aktionen werden via run-shell durchgereicht (statt
// SwitchScreenMsg), und nach erfolgreichem Dispatch quittet das
// Programm — damit das tmux-Popup zugeht. palette.WithStandalone()
// trägt das Verhalten.

import (
	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// PaletteDeps is the dependency bundle for the standalone
// `flow palette` subcommand. Screen baut den Palette-Model gegen den
// live-Pal aus tk.Load() — der Sidekick-Wiring nutzt dieselbe Factory-
// Form, daher kein Drift zwischen Popup und Tab.
type PaletteDeps struct {
	Screen func(tk.Palette) tea.Model
}

// NewPaletteCmd konstruiert das `flow palette` Cobra-Subcommand. Wird
// von der tmux-Plugin-Bridge aufgerufen:
//
//	bind "°" display-popup -E -T ' Palette ' -w 80% -h 80% \
//	     "zsh -c 'flow palette'"
//
// q/Ctrl+C bricht ab; Enter dispatched die Action und quittet
// (Standalone-Modus) bzw. lässt das Programm bei einem Fehler offen,
// damit der Danger-Toast lesbar bleibt.
func NewPaletteCmd(deps PaletteDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "palette",
		Short:        "Run the action palette (full-screen TUI for tmux popup)",
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
