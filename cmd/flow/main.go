// Package main is the flow CLI entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/adapter/cheatsheetfs"
	"github.com/serverkraken/flow/internal/adapter/dayoffstsv"
	"github.com/serverkraken/flow/internal/adapter/editor"
	"github.com/serverkraken/flow/internal/adapter/flockstate"
	"github.com/serverkraken/flow/internal/adapter/fspaletteentries"
	"github.com/serverkraken/flow/internal/adapter/fsprojects"
	"github.com/serverkraken/flow/internal/adapter/iniconfig"
	"github.com/serverkraken/flow/internal/adapter/jsonflowstate"
	"github.com/serverkraken/flow/internal/adapter/jsonpalettestats"
	"github.com/serverkraken/flow/internal/adapter/linkstsv"
	"github.com/serverkraken/flow/internal/adapter/systemclock"
	"github.com/serverkraken/flow/internal/adapter/tmuxbridge"
	"github.com/serverkraken/flow/internal/adapter/tsvsessions"
	"github.com/serverkraken/flow/internal/frontend/cli"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/markdown"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/cheatsheet"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/palette"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/projects"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
	kompfsstore "github.com/serverkraken/flow/internal/kompendium/adapter/fsstore"
	kompgitrepo "github.com/serverkraken/flow/internal/kompendium/adapter/gitrepo"
	kompgitsnapshot "github.com/serverkraken/flow/internal/kompendium/adapter/gitsnapshot"
	komplegacysource "github.com/serverkraken/flow/internal/kompendium/adapter/legacysource"
	kompnvimeditor "github.com/serverkraken/flow/internal/kompendium/adapter/nvimeditor"
	kompsqliteindex "github.com/serverkraken/flow/internal/kompendium/adapter/sqliteindex"
	komptarsnapshot "github.com/serverkraken/flow/internal/kompendium/adapter/tarsnapshot"
	kompdomain "github.com/serverkraken/flow/internal/kompendium/domain"
	kompendiumcli "github.com/serverkraken/flow/internal/kompendium/frontend/cli"
	kompbrowse "github.com/serverkraken/flow/internal/kompendium/frontend/tui/browse"
	kompusecase "github.com/serverkraken/flow/internal/kompendium/usecase"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/spf13/cobra"
)

// Paths bundles every filesystem location the dependency graph reads or writes.
// Tests rewire this against t.TempDir() so the whole graph runs in isolation.
type Paths struct {
	WorktimeLog        string
	TmuxDir            string
	CacheDir           string
	PluginsDir         string
	StateDir           string // ~/.local/state/flow — palette stats etc.
	Cheatsheet         string
	SourceCodeRoot     string // $SOURCECODE_ROOT or ~/Sourcecode — project enumeration root.
	KompendiumNotebook string // $NOTES_DIR or ~/notes — kompendium markdown notebook root.
	KompendiumIndex    string // $XDG_DATA_HOME/kompendium/index.db or ~/.local/share/kompendium/index.db.
}

// Deps is the wired dependency graph. K4.B extends it with the
// kompendium subtree's deps so `flow kompendium <verb>` (registered in
// K4.C) and the kompendium TUI screens (K5) can pull from a single
// build call.
type Deps struct {
	Worktime   cli.WorktimeDeps
	Sidekick   cli.SidekickDeps
	Cheatsheet cli.CheatsheetDeps
	Kompendium kompendiumcli.Deps
}

func buildDeps(p Paths) (Deps, func(), error) {
	clock := systemclock.New()
	tmux := tmuxbridge.New()

	sessionStore := tsvsessions.New(p.WorktimeLog)
	fileLock := flockstate.NewLock(filepath.Join(p.TmuxDir, "worktime.lock"))
	activeStore := flockstate.NewState(
		filepath.Join(p.TmuxDir, "worktime.state"),
		filepath.Join(p.TmuxDir, "worktime.pause"),
	)
	dayoffStore := dayoffstsv.New(
		filepath.Join(p.TmuxDir, "worktime-dayoffs.tsv"),
		filepath.Join(p.TmuxDir, "worktime-holidays.tsv"),
	)
	configReader := iniconfig.New(filepath.Join(p.TmuxDir, "worktime.conf"))
	linkStore := linkstsv.New(filepath.Join(p.TmuxDir, "worktime-links.tsv"))

	kompDeps, kompCleanup, err := buildKompendiumDeps(p, clock)
	if err != nil {
		return Deps{}, nil, err
	}

	pathOf := func(id string) string {
		parsed, perr := kompdomain.ParseID(id)
		if perr != nil {
			return ""
		}
		return kompDeps.Store.Path(parsed)
	}
	editorArgs := func(path string) ([]string, error) {
		cmd := kompDeps.EditCmd(path)
		return cmd.Args, nil
	}
	noteLauncher := editor.New(pathOf, editorArgs, envOr("FLOW_NOTE_VIEWER", "glow"))

	flowState := jsonflowstate.New(
		filepath.Join(p.CacheDir, "state.json"),
		filepath.Join(p.CacheDir, "next-screen"),
	)
	cheatsheetReader := cheatsheetfs.New(p.Cheatsheet)
	mdRenderer := markdown.NewRenderer()
	paletteEntries := fspaletteentries.New(
		p.PluginsDir,
		filepath.Join(p.TmuxDir, "enabled-plugins"),
	)
	paletteStats := jsonpalettestats.New(filepath.Join(p.StateDir, "palette-stats.json"))
	projectScanner := fsprojects.New(p.SourceCodeRoot)

	targets := &usecase.TargetResolver{Config: configReader, DayOffs: dayoffStore, DefaultTarget: 8 * time.Hour}
	reader := &usecase.WorktimeReader{Sessions: sessionStore, State: activeStore, Targets: targets, Clock: clock}
	stats := &usecase.StatsComputer{
		Reader:  reader,
		Targets: targets,
		DayOffs: dayoffStore,
		State:   activeStore,
	}
	reporter := &usecase.Reporter{
		Reader:  reader,
		DayOffs: dayoffStore,
		Targets: targets,
		Stats:   stats,
		Clock:   clock,
	}
	sessionWriter := &usecase.SessionWriter{
		Sessions: sessionStore,
		State:    activeStore,
		Lock:     fileLock,
		Reader:   reader,
		Clock:    clock,
	}
	statusComposer := &usecase.StatusComposer{
		Reader:  reader,
		DayOffs: dayoffStore,
		Targets: targets,
		Stats:   stats,
		Tmux:    tmux,
		Clock:   clock,
	}
	dayoffWriter := &usecase.DayOffWriter{Store: dayoffStore}
	dayoffReader := &usecase.DayOffReader{Store: dayoffStore}
	tagger := &usecase.Tagger{Sessions: sessionStore}
	linkReader := &usecase.LinkReader{Store: linkStore}
	linkWriter := &usecase.LinkWriter{Store: linkStore}
	noteOpener := &usecase.NoteOpener{Launcher: noteLauncher}
	paletteReader := &usecase.PaletteReader{
		Entries: paletteEntries,
		Stats:   paletteStats,
		Tmux:    tmux,
		Clock:   clock,
	}
	paletteWriter := &usecase.PaletteWriter{Stats: paletteStats, Clock: clock}
	projectsReader := &usecase.ProjectsReader{Scanner: projectScanner, Tmux: tmux}
	projectSwitcher := &usecase.ProjectSwitcher{Tmux: tmux}

	// Hoisted so both the sidekick worktime tab and the standalone
	// `flow worktime today` verb share one factory — same wiring, no
	// drift risk.
	worktimeScreen := func(pal theme.Palette) tea.Model {
		return worktime.New(pal, worktime.Deps{
			Reader:        reader,
			Stats:         stats,
			SessionWriter: sessionWriter,
			Tagger:        tagger,
			DayOffReader:  dayoffReader,
			DayOffWriter:  dayoffWriter,
			LinkReader:    linkReader,
			LinkWriter:    linkWriter,
			Reporter:      reporter,
			NoteOpener:    noteOpener,
			Clock:         clock,
		})
	}

	return Deps{
		Worktime: cli.WorktimeDeps{
			Clock:          clock,
			Tmux:           tmux,
			SessionWriter:  sessionWriter,
			StatusComposer: statusComposer,
			Reporter:       reporter,
			Stats:          stats,
			DayOffWriter:   dayoffWriter,
			DayOffReader:   dayoffReader,
			Reader:         reader,
			Screen:         worktimeScreen,
		},
		Sidekick: cli.SidekickDeps{
			FlowState: flowState,
			Cheatsheet: func(pal theme.Palette) tea.Model {
				return cheatsheet.New(pal, cheatsheetReader, mdRenderer)
			},
			Palette: func(pal theme.Palette) tea.Model {
				return palette.New(pal, paletteReader, paletteWriter, tmux)
			},
			Projects: func(pal theme.Palette) tea.Model {
				return projects.New(pal, p.SourceCodeRoot, projectsReader, projectSwitcher)
			},
			Worktime: worktimeScreen,
			Notes: func(_ theme.Palette) tea.Model {
				return buildNotesScreen(p, kompDeps)
			},
		},
		// Standalone-Cheatsheet teilt sich Reader und Renderer mit dem
		// Sidekick-Tab — identische Render-Pipeline, identische Theme,
		// keine Drift zwischen Popup und Tab.
		Cheatsheet: cli.CheatsheetDeps{
			Reader:   cheatsheetReader,
			Renderer: mdRenderer,
		},
		Kompendium: kompDeps,
	}, kompCleanup, nil
}

// buildNotesScreen constructs the kompendium browse model wired into
// flow's sidekick. The browse model has its own theme (kompendium's
// 22-field Tokyonight Night) — it consumes flow's theme.Palette only
// for sizing/chrome harmony, not colour selection. The wikilink
// resolver, edit Cmd, and write Cmd all reuse what kompDeps already
// has. currentRepo is detected from the launch cwd; when flow lives
// outside a git repo the project promotion just stays off.
func buildNotesScreen(p Paths, kompDeps kompendiumcli.Deps) tea.Model {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	var currentRepo kompdomain.CanonicalURL
	if info, derr := kompDeps.Repo.Detect(context.Background(), cwd); derr == nil {
		currentRepo = info.URL
	}
	writeCmd := func() *exec.Cmd {
		exe, e := os.Executable()
		if e != nil || exe == "" {
			exe = "flow"
		}
		c := exec.Command(exe, "kompendium", "write", "--cwd", cwd)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c
	}
	m := kompbrowse.New(
		kompDeps.ListNotes,
		kompDeps.Store,
		kompDeps.DeleteNote,
		currentRepo,
		kompDeps.EditCmd,
		writeCmd,
	)
	if p.KompendiumIndex != "" {
		m = m.WithIndexAge(func() time.Time {
			st, e := os.Stat(p.KompendiumIndex)
			if e != nil {
				return time.Time{}
			}
			return st.ModTime()
		})
	}
	if kompDeps.RenderBacklinks != nil {
		m = m.WithBacklinks(func(id kompdomain.ID) []kompusecase.BacklinkRef {
			out, berr := kompDeps.RenderBacklinks.Execute(context.Background(), kompusecase.RenderBacklinksInput{NoteID: id})
			if berr != nil {
				return nil
			}
			return out.Backlinks
		})
	}
	return m
}

// buildKompendiumDeps wires every kompendium-subtree adapter behind its
// port and assembles the use cases the CLI subcommand tree consumes.
// Returns a cleanup that releases the sqlite indexer handle; main()
// defers it so the FTS5 WAL gets a clean checkpoint on exit.
func buildKompendiumDeps(p Paths, clock systemclock.Clock) (kompendiumcli.Deps, func(), error) {
	store, err := kompfsstore.New(p.KompendiumNotebook)
	if err != nil {
		return kompendiumcli.Deps{}, nil, fmt.Errorf("kompendium notebook: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(p.KompendiumIndex), 0o755); err != nil {
		return kompendiumcli.Deps{}, nil, fmt.Errorf("kompendium index dir: %w", err)
	}
	indexer, err := kompsqliteindex.New(p.KompendiumIndex)
	if err != nil {
		return kompendiumcli.Deps{}, nil, fmt.Errorf("kompendium index: %w", err)
	}
	cleanup := func() { _ = indexer.Close() }

	repo := kompgitrepo.New()
	nvim := kompnvimeditor.New()
	notebookGit := kompgitsnapshot.New()
	tar := komptarsnapshot.New()

	createDaily := kompusecase.NewCreateDaily(store, clock, nvim)
	createDaily.Index = indexer
	createProject := kompusecase.NewCreateProject(store, repo, clock, nvim)
	createProject.Index = indexer
	createFree := kompusecase.NewCreateFree(store, nvim)
	createFree.Index = indexer
	captureDaily := kompusecase.NewCaptureDaily(store, clock)
	captureDaily.Index = indexer
	open := kompusecase.NewOpen(store, nvim)
	open.Index = indexer
	importLegacy := kompusecase.NewImportLegacy(store, komplegacysource.New())
	importLegacy.Index = indexer

	return kompendiumcli.Deps{
		Store:            store,
		Repo:             repo,
		CreateDaily:      createDaily,
		CreateProject:    createProject,
		CreateFree:       createFree,
		CaptureDaily:     captureDaily,
		Open:             open,
		ListNotes:        kompusecase.NewListNotes(store),
		SearchNotes:      kompusecase.NewSearchNotes(indexer),
		RenderDaily:      kompusecase.NewRenderDaily(store),
		RenderBacklinks:  kompusecase.NewRenderBacklinks(store, indexer),
		InitNotebook:     kompusecase.NewInitNotebook(store, notebookGit),
		SnapshotNotebook: kompusecase.NewSnapshotNotebook(store, notebookGit),
		ExportTar:        kompusecase.NewExportTar(store, tar),
		ImportTar:        kompusecase.NewImportTar(store, tar),
		ExportBundle:     kompusecase.NewExportBundle(store, notebookGit),
		ImportBundle:     kompusecase.NewImportBundle(store, notebookGit),
		SyncNotebook:     kompusecase.NewSyncNotebook(store, notebookGit),
		ManageRemote:     kompusecase.NewManageRemote(store, notebookGit),
		Doctor:           kompusecase.NewDoctor(store, notebookGit),
		ImportLegacy:     importLegacy,
		RebuildIndex:     kompusecase.NewRebuildIndex(store, indexer),
		DeleteNote:       kompusecase.NewDeleteNote(store, indexer),
		EditCmd:          nvim.Cmd,
		IndexPath:        p.KompendiumIndex,
	}, cleanup, nil
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

var rootCmd = &cobra.Command{
	Use:          "flow",
	Short:        "Workspace TUI sidekick",
	SilenceUsage: true,
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to get home dir:", err)
		os.Exit(1)
	}
	tmuxDir := filepath.Join(home, ".tmux")
	sourceRoot := os.Getenv("SOURCECODE_ROOT")
	if sourceRoot == "" {
		sourceRoot = filepath.Join(home, "Sourcecode")
	}
	notebookRoot := os.Getenv("NOTES_DIR")
	if notebookRoot == "" {
		notebookRoot = filepath.Join(home, "notes")
	}
	indexDir := os.Getenv("XDG_DATA_HOME")
	if indexDir == "" {
		indexDir = filepath.Join(home, ".local", "share")
	}
	indexPath := filepath.Join(indexDir, "kompendium", "index.db")

	deps, cleanup, err := buildDeps(Paths{
		WorktimeLog:        filepath.Join(tmuxDir, "worktime.log"),
		TmuxDir:            tmuxDir,
		CacheDir:           filepath.Join(home, ".cache", "flow"),
		PluginsDir:         filepath.Join(tmuxDir, "plugins"),
		StateDir:           filepath.Join(home, ".local", "state", "flow"),
		Cheatsheet:         filepath.Join(tmuxDir, "cheatsheet.md"),
		SourceCodeRoot:     sourceRoot,
		KompendiumNotebook: notebookRoot,
		KompendiumIndex:    indexPath,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer cleanup()

	rootCmd.AddCommand(cli.NewSidekickCmd(deps.Sidekick))
	rootCmd.AddCommand(cli.NewWorktimeCmd(deps.Worktime))
	rootCmd.AddCommand(cli.NewCheatsheetCmd(deps.Cheatsheet))
	rootCmd.AddCommand(kompendiumcli.NewRootCmd(deps.Kompendium))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
