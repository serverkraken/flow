// Package cli implements the kompendium command-line frontend on top of
// cobra. Subcommands are added by the composition root in
// cmd/kompendium/main.go via the Deps struct, never inside this package, so
// business logic stays out of the frontend layer.
package cli

import (
	"io"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// Version is the kompendium binary version. Release builds override it via
// -ldflags "-X github.com/serverkraken/flow/internal/kompendium/frontend/cli.Version=..."
const Version = "v0.0.0-dev"

// Deps bundles every dependency the CLI subcommands need. The composition
// root constructs the use cases over real adapters; tests substitute
// in-memory fakes from internal/testutil.
type Deps struct {
	Store            ports.NoteStore
	Rooter           ports.NotebookRooter // optional — only wired for local fsstore
	Repo             ports.RepoDetector
	CreateDaily      *usecase.CreateDaily
	CreateProject    *usecase.CreateProject
	CreateFree       *usecase.CreateFree
	CaptureDaily     *usecase.CaptureDaily
	Open             *usecase.Open
	ListNotes        *usecase.ListNotes
	SearchNotes      *usecase.SearchNotes
	RenderDaily      *usecase.RenderDaily
	RenderBacklinks  *usecase.RenderBacklinks
	InitNotebook     *usecase.InitNotebook
	SnapshotNotebook *usecase.SnapshotNotebook
	ExportTar        *usecase.ExportTar
	ImportTar        *usecase.ImportTar
	ExportBundle     *usecase.ExportBundle
	ImportBundle     *usecase.ImportBundle
	SyncNotebook     *usecase.SyncNotebook
	ManageRemote     *usecase.ManageRemote
	Doctor           *usecase.Doctor
	ImportLegacy     *usecase.ImportLegacy
	RebuildIndex     *usecase.RebuildIndex
	DeleteNote       *usecase.DeleteNote
	EditNote         *usecase.EditNote

	// EditCmd builds an unstarted *exec.Cmd value the browse TUI hands
	// to tea.ExecProcess on Enter to launch nvim. The composition root
	// provides it so the TUI never needs to import adapter/* directly.
	// Read-only viewing (`v`) is handled in-process by
	// internal/frontend/tui/view, no Cmd needed.
	EditCmd func(path string) *exec.Cmd

	// IndexPath is the on-disk path to the SQLite FTS5 index. Empty means
	// "no index wired" — the browse status bar drops the "index Nm"
	// segment when this is unset or the file is missing.
	IndexPath string
}

// NewRootCmd returns a freshly built root cobra command with every
// subcommand attached. Each call yields an independent command tree so tests
// stay free of shared state.
func NewRootCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "kompendium",
		Short:         "Personal Markdown notes & wiki — daily, project, free",
		Long:          "kompendium consolidates daily and project notes into a single Markdown notebook with full-text search, wikilinks, and git-backed portability.",
		Version:       Version,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		newNewCmd(deps),
		newTodayCmd(deps),
		newCaptureCmd(deps),
		newLsCmd(deps),
		newSearchCmd(deps),
		newPathCmd(deps),
		newOpenCmd(deps),
		newDailyRenderCmd(deps),
		newInitCmd(deps),
		newSnapshotCmd(deps),
		newExportCmd(deps),
		newImportCmd(deps),
		newRemoteCmd(deps),
		newSyncCmd(deps),
		newDoctorCmd(deps),
		newImportLegacyCmd(deps),
		newBrowseCmd(deps),
		newWriteCmd(deps),
		newIndexCmd(deps),
		newVersionCmd(),
		// (Bundle commands are flags on export/import, not separate subcommands.)
	)
	return cmd
}

// Execute parses args against the root command and writes output to the
// given writers. The function never reads os.Args or writes to os.Stdout /
// os.Stderr directly so callers (main, tests) stay in full control.
func Execute(args []string, out, errOut io.Writer, deps Deps) error {
	cmd := NewRootCmd(deps)
	cmd.SetArgs(args)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	return cmd.Execute()
}
