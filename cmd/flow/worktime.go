package main

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui"
	"github.com/spf13/cobra"
)

func worktimeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "worktime",
		Short: "Worktime timer (TUI)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			// slog/stderr must never corrupt the TUI: send logs to a file.
			logf, err := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
				os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err == nil {
				defer func() { _ = logf.Close() }()
				os.Stderr = logf
			}
			m := tui.New(client, os.Getenv("USER"))
			_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
			return err
		},
	}
}
