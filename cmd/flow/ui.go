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

func uiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ui",
		Short: "Sidekick-Shell (TUI) — alle Screens in einem Programm",
		RunE:  runUI,
	}
}

func runUI(cmd *cobra.Command, _ []string) error {
	client, err := clientFromStore(cmd.Context())
	if err != nil {
		return err
	}
	logf, err := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		defer func() { _ = logf.Close() }()
		os.Stderr = logf
	}
	pal := theme.Load()
	m := shell.New(client, os.Getenv("USER"), pal).
		WithTabs([]shell.Route{
			shell.NewHomeRoute(client, pal, os.Getenv("USER")),
			worktime.NewTodayRoute(client, time.Now, pal, worktime.BuildRegistry(client, pal)),
		})
	_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
	return err
}
