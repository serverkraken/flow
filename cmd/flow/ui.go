package main

import (
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/editor"
	"github.com/serverkraken/flow/internal/adapter/opener"
	"github.com/serverkraken/flow/internal/tui/screen/worktime"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	docsscreen "github.com/serverkraken/flow/internal/tui/screen/docs"
	projectscreen "github.com/serverkraken/flow/internal/tui/screen/projects"
	"github.com/spf13/cobra"
)

func uiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ui [tab]",
		Short: "Sidekick-Shell (TUI) — Home · Worktime · Docs · Projekte in einem Programm",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runUI,
	}
}

// tabIndexForArg maps an optional positional arg to a start-tab index
// (0=Home, 1=Worktime, 2=Docs, 3=Projekte); unknown/empty → Home.
func tabIndexForArg(args []string) int {
	if len(args) == 0 {
		return 0
	}
	switch args[0] {
	case "worktime", "work", "w":
		return 1
	case "docs", "doc", "d":
		return 2
	case "projekte", "projects", "p":
		return 3
	default:
		return 0
	}
}

func runUI(cmd *cobra.Command, args []string) error {
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
	user := os.Getenv("USER")
	adapted := engagementCreateClient{client}
	m := shell.New(client, user, pal).
		WithTabs([]shell.Route{
			shell.NewHomeRoute(client, pal, user),
			worktime.NewTodayRoute(client, time.Now, pal, worktime.BuildRegistry(client, pal)),
			docsscreen.NewRoute(client, editor.New(), opener.New(), pal, user),
			projectscreen.MountWithAPI(client, client, adapted, pal, user),
		}).
		WithActiveTab(tabIndexForArg(args))
	_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
	return err
}
