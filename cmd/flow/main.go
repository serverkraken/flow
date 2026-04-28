// Package main is the flow CLI entry point.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/app"
	"github.com/serverkraken/flow/internal/state"
	tk "github.com/serverkraken/tui-kit/theme"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "flow",
	Short:        "Workspace TUI sidekick",
	SilenceUsage: true,
}

var sidekickCmd = &cobra.Command{
	Use:          "sidekick",
	Short:        "Run as tmux sidekick panel",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		tk.Init()
		p := tk.Load()

		s := state.Load()
		if next := state.CheckNextScreen(); next != "" {
			s.Screen = next
			s.Filter = ""
			s.Cursor = 0
		} else {
			s.Screen = state.Palette
		}

		m := app.New(p, s)
		prog := tea.NewProgram(m, tea.WithAltScreen())
		final, err := prog.Run()
		if err != nil {
			return err
		}
		if am, ok := final.(app.Model); ok {
			_ = state.Save(am.CurrentState())
		}
		return nil
	},
}

func main() {
	rootCmd.AddCommand(sidekickCmd)
	worktimeCmd.AddCommand(wtStatusCmd, wtStartCmd, wtStopCmd)
	rootCmd.AddCommand(worktimeCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
