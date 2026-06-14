package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui"
	"github.com/spf13/cobra"
)

func worktimeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "worktime",
		Short: "Worktime timer (TUI)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base := envOr("FLOW_SERVER_URL", "http://localhost:8080")
			token := os.Getenv("FLOW_TOKEN") // device-flow login lands in M1b
			if token == "" {
				return fmt.Errorf("set FLOW_TOKEN (device-flow login comes in M1b)")
			}
			// slog/stderr must never corrupt the TUI: send logs to a file.
			logf, err := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
				os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err == nil {
				defer func() { _ = logf.Close() }()
				os.Stderr = logf
			}
			client := apiclient.New(base, token)
			m := tui.New(client, os.Getenv("USER"))
			_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
			return err
		},
	}
}
