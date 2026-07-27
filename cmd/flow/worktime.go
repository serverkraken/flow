package main

import (
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/screen/worktime"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/spf13/cobra"
)

func worktimeCmd() *cobra.Command {
	cmd := &cobra.Command{
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
			pal := theme.Load()
			m := shell.New(client, os.Getenv("USER"), pal).
				WithTabs([]shell.Route{
					worktime.NewTodayRoute(client, time.Now, pal, worktime.BuildRegistry(client, pal)),
				})
			_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
			return err
		},
	}
	cmd.AddCommand(worktimeImportCmd(), worktimeStatusCmd(), worktimeStopCmd())
	return cmd
}
