// Package main is the flow CLI entry point.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/cheatsheetfs"
	"github.com/serverkraken/flow/internal/adapter/mutexlock"
	"github.com/serverkraken/flow/internal/adapter/fspaletteentries"
	"github.com/serverkraken/flow/internal/adapter/fssourcedirs"
	"github.com/serverkraken/flow/internal/adapter/gitremote"
	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/adapter/iniconfig"
	"github.com/serverkraken/flow/internal/adapter/jsonflowstate"
	"github.com/serverkraken/flow/internal/adapter/jsonpalettestats"
	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/adapter/linkstsv"
	"github.com/serverkraken/flow/internal/adapter/oidcclient"
	"github.com/serverkraken/flow/internal/adapter/output"
	"github.com/serverkraken/flow/internal/adapter/systemclock"
	"github.com/serverkraken/flow/internal/adapter/tmuxbridge"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/cli"
	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/markdown"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/cheatsheet"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/palette"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/projects"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	kompapistore "github.com/serverkraken/flow/internal/kompendium/adapter/apistore"
	kompgitrepo "github.com/serverkraken/flow/internal/kompendium/adapter/gitrepo"
	kompnvimeditor "github.com/serverkraken/flow/internal/kompendium/adapter/nvimeditor"
	kompdomain "github.com/serverkraken/flow/internal/kompendium/domain"
	kompendiumcli "github.com/serverkraken/flow/internal/kompendium/frontend/cli"
	kompbrowse "github.com/serverkraken/flow/internal/kompendium/frontend/tui/browse"
	kompwritepicker "github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
	kompusecase "github.com/serverkraken/flow/internal/kompendium/usecase"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/spf13/cobra"
)

// version is overridden at build time via -ldflags "-X main.version=…".
var version = "dev"

// Paths bundles every filesystem location the dependency graph reads or writes.
// Tests rewire this against t.TempDir() so the whole graph runs in isolation.
type Paths struct {
	Home                string // user home (resolves ~/Downloads for the worktime menu's file output target).
	WorktimeLog         string
	TmuxDir             string
	CacheDir            string
	PluginsDir          string
	StateDir            string // ~/.local/state/flow — palette stats etc.
	Cheatsheet          string
	SourceCodeRoot      string // $SOURCECODE_ROOT or ~/Sourcecode — project enumeration root.
	KompendiumNotebook  string // $NOTES_DIR or ~/notes — kompendium markdown notebook root.
	KompendiumIndexPath string // $XDG_DATA_HOME/kompendium/index.db or ~/.local/share/kompendium/index.db.
}

// Env bundles configuration values resolved from environment variables.
// Per review finding A1, every os.Getenv call for *app configuration*
// lives in main() so deeper layers (adapters, screens) get their values
// via constructor params and stay testable without env mutation.
//
// Platform-detection probes are the documented exception: variables
// that aren't part of the app's config surface but describe the
// runtime host (TMUX, COLORTERM, VISUAL, EDITOR). They stay where they
// are read because (a) they aren't user-configurable in the
// flow-config sense, (b) hoisting them to main.go would just turn
// main.go into a host-probe registry, and (c) the test boundary for
// host-detection logic is the function under test, not the env. The
// three current sites are:
//
//   - internal/frontend/tui/theme/load.go  — TMUX, COLORTERM
//   - internal/frontend/tui/components/markdown_overlay/code_copy.go — TMUX
//   - internal/kompendium/adapter/nvimeditor/split.go — VISUAL, EDITOR
//
// Adding a new platform probe? Note it in the list above so the
// exception stays auditable.
type Env struct {
	WorktimeTargetHours time.Duration // $WORKTIME_TARGET_HOURS as duration (0 → adapter falls back to 8h)
	WorktimeLand        string        // $WORKTIME_LAND, the dayoff Bundesland default
	ServerURL           string        // $FLOW_SERVER_URL — flow-server base URL
	OIDCClientID        string        // $FLOW_OIDC_CLIENT_ID — OIDC client id used for token refresh (default "flow-cli")
}

// Deps is the wired dependency graph. K4.B extends it with the
// kompendium subtree's deps so `flow kompendium <verb>` (registered in
// K4.C) and the kompendium TUI screens (K5) can pull from a single
// build call.
type Deps struct {
	Worktime   cli.WorktimeDeps
	Sidekick   cli.SidekickDeps
	Cheatsheet cli.CheatsheetDeps
	Palette    cli.PaletteDeps
	Projects   cli.ProjectsDeps
	Repo       cli.RepoDeps
	Kompendium kompendiumcli.Deps
	Docs       DocsDeps
}

func buildDeps(ctx context.Context, p Paths, env Env) (Deps, func(), error) {
	clock := systemclock.New()
	tmux := tmuxbridge.New()

	configReader := iniconfig.New(filepath.Join(p.TmuxDir, "worktime.conf"), env.WorktimeTargetHours)
	linkStore := linkstsv.New(filepath.Join(p.TmuxDir, "worktime-links.tsv"))
	outputTargets := output.New(p.Home, tmux)

	// httpapi client — single client shared by all resource adapters.
	serverURL := env.ServerURL
	keyring := keyringadapter.New()
	keyringSlot := "tokens:" + serverURL
	client := httpapi.New(httpapi.Config{
		BaseURL: serverURL,
		Tokens:  keyring,
		Slot:    keyringSlot,
		Version: version,
		Refresher: &oidcclient.StoreRefresher{
			ServerURL: serverURL,
			ClientID:  env.OIDCClientID,
			Store:     keyring,
			Slot:      keyringSlot,
		},
	})

	// Resource adapters.
	httpProjects := httpapi.NewProjects(client)
	httpSessions := httpapi.NewSessions(client)
	httpActive := httpapi.NewActiveSessions(client)
	httpMachine := httpapi.NewMachine(client, httpActive, httpSessions)
	httpDocuments := httpapi.NewDocuments(client)
	httpDayOffs := httpapi.NewDayOffs(client)
	httpIdentity := httpapi.NewIdentity(client)

	// SSE events: invalidate per-resource caches and wake the TUI on
	// server-side changes. Each adapter exposes Invalidate() which marks
	// its internal cache stale so the next read triggers a fresh server fetch.
	// invalidateKomp is assigned after buildKompendiumDeps returns the store;
	// the closure captures the pointer so the late assignment is visible.
	var invalidateKomp func()
	invalidateFn := func(resource string) {
		switch resource {
		case "worktime":
			httpActive.Invalidate()
			httpSessions.Invalidate()
		case "projects":
			httpProjects.Invalidate()
		case "dayoffs":
			httpDayOffs.Invalidate()
		case "documents":
			if invalidateKomp != nil {
				invalidateKomp()
			}
		}
	}
	events := httpapi.NewEvents(client, invalidateFn)
	events.Start(ctx)

	// One-time meta check to detect server version and outdated client.
	go func() { _ = client.CheckMeta(ctx) }()

	// Identity: resolve the logged-in user from the bearer API.
	// On ErrTokenNotFound the user is logged out — run with empty userID.
	identityUC := usecase.NewIdentity(httpIdentity)
	localUser, identErr := identityUC.ResolveActiveUser(ctx)
	userID := ""
	if identErr == nil {
		userID = localUser.ID
	} else if !errors.Is(identErr, ports.ErrTokenNotFound) {
		slog.Warn("flow: identity resolve failed — running logged-out", slog.Any("err", identErr))
	}

	// Use cases backed by httpapi adapters.
	projectsUC := usecase.NewProjects(nil, httpProjects, nil)
	sessionsUC := usecase.NewSessions(nil, httpProjects, httpSessions, nil)
	activeSessionsUC := usecase.NewActiveSessions(nil, httpProjects, httpActive, httpMachine)

	// RepoNotes use case: backed by the Documents API via RepoDocAdapter.
	// RepoDocAdapter maps ports.RepoStore + ports.RepoNoteStore onto the
	// /api/v1/repos/<key>/note endpoint so usecase.RepoNotes works unchanged.
	repoDocShim := httpapi.NewRepoDocAdapter(httpDocuments)
	repoNotesUC := usecase.NewRepoNotes(repoDocShim.RepoStore(), repoDocShim.RepoNoteStore(), nil /* no queue in server mode */, gitremote.New())

	kompDeps, kompStore, kompCleanup, err := buildKompendiumDeps(httpDocuments, userID, clock)
	if err != nil {
		return Deps{}, nil, err
	}
	// Wire the documents-SSE invalidation now that the store is available.
	invalidateKomp = kompStore.Invalidate

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
	projectScanner := fssourcedirs.New(p.SourceCodeRoot)

	targets := &usecase.TargetResolver{Config: configReader, DayOffs: httpDayOffs, DefaultTarget: 8 * time.Hour}
	reader := &usecase.WorktimeReader{
		Sessions: httpSessions,
		Active:   httpActive,
		Projects: httpProjects,
		UserID:   userID,
		Targets:  targets,
		Clock:    clock,
	}
	stats := &usecase.StatsComputer{
		Reader:  reader,
		Targets: targets,
		DayOffs: httpDayOffs,
	}
	reporter := &usecase.Reporter{
		Reader:  reader,
		DayOffs: httpDayOffs,
		Targets: targets,
		Stats:   stats,
		Clock:   clock,
	}
	statusComposer := &usecase.StatusComposer{
		Reader:  reader,
		DayOffs: httpDayOffs,
		Targets: targets,
		Stats:   stats,
		Tmux:    tmux,
		Clock:   clock,
		Config:  configReader,
	}
	dayoffWriter := &usecase.DayOffWriter{Store: httpDayOffs}
	tagger := &usecase.Tagger{Sessions: httpSessions}
	// sessionWriter backs the tag/note/edit/delete paths in TUI and CLI.
	// State is nil because lifecycle (Start/Stop/Pause/Resume) goes via
	// ActiveSessions in server mode; this writer is edit-only in that path.
	sessionWriter := &usecase.SessionWriter{
		Sessions: httpSessions,
		State:    nil, // server mode: lifecycle goes via ActiveSessions; edit-only
		Lock:     mutexlock.New(),
		Reader:   reader,
		Clock:    clock,
		UserID:   userID,
	}
	linkReader := &usecase.LinkReader{Store: linkStore}
	linkWriter := &usecase.LinkWriter{Store: linkStore}
	noteOpener := &usecase.NoteOpener{Launcher: &editNoteOpener{uc: kompDeps.EditNote}}
	currentRepo := detectCurrentRepo(kompDeps)
	noteLister := newKompendiumNoteLister(kompDeps, currentRepo)
	noteReader := newKompendiumNoteReader(kompDeps)
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
			Reader:           reader,
			Stats:            stats,
			SessionWriter:    sessionWriter, // server mode: lifecycle via ActiveSessions; tag/note/edit/delete via sessionWriter
			Tagger:           tagger,
			DayOffStore:      httpDayOffs,
			DayOffWriter:     dayoffWriter,
			LinkReader:       linkReader,
			LinkWriter:       linkWriter,
			Reporter:         reporter,
			NoteOpener:       noteOpener,
			NoteLister:       noteLister,
			NoteReader:       noteReader,
			MarkdownRenderer: mdRenderer,
			Clock:            clock,
			Output:           outputTargets,
			HomeDir:          p.Home,
			Land:             env.WorktimeLand,
			Projects:         projectsUC,
			ActiveSessions:   activeSessionsUC,
			UserID:           userID,
			Changed:          events.Changed(),
			Status:           client.StatusOf().Snapshot,
		})
	}

	return Deps{
			Worktime: cli.WorktimeDeps{
				Clock:          clock,
				Tmux:           tmux,
				SessionWriter:  sessionWriter, // server mode: edit methods only; lifecycle goes via ActiveSessions
				StatusComposer: statusComposer,
				Reporter:       reporter,
				Stats:          stats,
				DayOffWriter:   dayoffWriter,
				DayOffStore:    httpDayOffs,
				Reader:         reader,
				Screen:         worktimeScreen,

				UserID: userID,
				ResolveProject: func(userID, explicitID, pwd string) (domain.Project, error) {
					return sessionsUC.ResolveProject(userID, explicitID, pwd)
				},
				StartActiveSession: func(userID, projectID, tag, note string) (domain.ActiveSession, error) {
					return activeSessionsUC.Start(userID, projectID, tag, note)
				},
				StopActiveSession: func(userID, projectID, tag, note string) (domain.Session, error) {
					return activeSessionsUC.Stop(userID, projectID, tag, note)
				},
				ListActiveSessions: func(userID string) ([]domain.ActiveSession, error) {
					return activeSessionsUC.ListActive(userID)
				},
				Migrate:     nil, // TSV migration not available in server mode
				PauseMarker: nil, // no local pause state in server mode
				CorrectActiveStart: func(userID string, ts time.Time) error {
					return activeSessionsUC.CorrectStart(userID, ts)
				},
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
					return projects.NewWithDeps(pal, p.SourceCodeRoot, projectsReader, projectSwitcher, projectsUC, httpSessions, userID)
				},
				Worktime: worktimeScreen,
				Notes: func(pal theme.Palette) tea.Model {
					return buildNotesScreen(p, pal, kompDeps, currentRepo)
				},
			},
			// Standalone-Cheatsheet teilt sich Reader und Renderer mit dem
			// Sidekick-Tab — identische Render-Pipeline, identische Theme,
			// keine Drift zwischen Popup und Tab.
			Cheatsheet: cli.CheatsheetDeps{
				Reader:   cheatsheetReader,
				Renderer: mdRenderer,
			},
			// Standalone-Palette für `flow palette` — tmux-display-popup-
			// Aufruf (CLAUDE-tmux-migration-plan.md). WithStandalone()
			// schaltet die Dispatch-Semantik um (goto.sh → run-shell statt
			// SwitchScreenMsg) und quittet nach erfolgreichem Dispatch.
			Palette: cli.PaletteDeps{
				Screen: func(pal theme.Palette) tea.Model {
					return palette.New(pal, paletteReader, paletteWriter, tmux, palette.WithStandalone())
				},
			},
			// Standalone-Projects für `flow projects`. tmux switch-client
			// hängt den Client um, nach Erfolg quittet die TUI — identisch
			// zum Sidekick-Verhalten, daher genügt die API-Symmetrie via
			// projects.WithStandalone(). CRUD subcommands (list/create/rename/archive)
			// are wired via the CRUD field; the Screen launcher is unchanged.
			Projects: cli.ProjectsDeps{
				Screen: func(pal theme.Palette) tea.Model {
					return projects.NewWithDeps(pal, p.SourceCodeRoot, projectsReader, projectSwitcher, projectsUC, httpSessions, userID, projects.WithStandalone())
				},
				CRUD: &cli.ProjectsCRUDDeps{
					Projects: projectsUC,
					UserID:   userID,
				},
				ProjectStore: httpProjects,
			},
			Repo: cli.RepoDeps{
				UserID: userID,
				Notes:  repoNotesUC,
			},
			Kompendium: kompDeps,
			Docs: DocsDeps{
				Docs:   httpDocuments,
				UserID: userID,
			},
		}, func() {
			events.Stop()
			kompCleanup()
		}, nil
}

// buildNotesScreen constructs the kompendium browse model wired into
// flow's sidekick. The browse model has its own theme (kompendium's
// 22-field Tokyonight Night) — it consumes flow's theme.Palette only
// for sizing/chrome harmony, not colour selection. The wikilink
// resolver, edit Cmd, and write Cmd all reuse what kompDeps already
// has. currentRepo is detected from the launch cwd; when flow lives
// outside a git repo the project promotion just stays off.
func buildNotesScreen(p Paths, pal theme.Palette, kompDeps kompendiumcli.Deps, currentRepo kompdomain.CanonicalURL) tea.Model {
	// Sidekick-Notes-Tab: pal kommt vom Sidekick-Root (tk.Load() in
	// cli/sidekick.go) durch — markdown_overlay und writepicker
	// behalten ihre SetPalette-Bridges (zwei eigenständige Refactors
	// stehen noch aus), kompbrowse läuft seit Phase 6 per-Model:
	// pal wird als erstes New()-Arg übergeben und in newBrowseStyles
	// gecached, kein Package-State mehr.
	markdown_overlay.SetPalette(pal)
	kompwritepicker.SetPalette(pal)

	// currentRepo is detected once in buildDeps and threaded in here —
	// previously this function called os.Getwd + Repo.Detect a second
	// time, duplicating the work and risking drift. (Polish item.)
	// writeCmd builds the `flow kompendium new <type>` cmd that runs
	// after the in-process picker harvested a Result. Dispatch lives
	// in kompendiumcli so both the standalone `flow kompendium browse`
	// path and this sidekick-embedded path share one factory — adding
	// a fourth picker choice should not require touching main.go.
	// cwd is resolved at click-time inside the factory so the user's
	// pane CWD wins even if it changed since startup.
	cwd, _ := os.Getwd()
	writeCmd := kompendiumcli.BuildWriteCmd(cwd)
	m := kompbrowse.New(
		pal,
		kompDeps.ListNotes,
		kompDeps.Store,
		kompDeps.DeleteNote,
		currentRepo,
		kompDeps.EditCmd,
		writeCmd,
	)
	if p.KompendiumIndexPath != "" {
		m = m.WithIndexAge(func() time.Time {
			st, e := os.Stat(p.KompendiumIndexPath)
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

// buildKompendiumDeps wires the kompendium-subtree adapters against the
// server-side DocumentStore HTTP API. The returned *kompapistore.Store is
// exposed so the caller can wire its Invalidate method into the SSE handler.
func buildKompendiumDeps(
	docs ports.DocumentStore,
	userID string,
	clock systemclock.Clock,
) (kompendiumcli.Deps, *kompapistore.Store, func(), error) {
	store := kompapistore.New(docs, userID)
	repo := kompgitrepo.New()
	nvim := kompnvimeditor.New()

	createDaily := kompusecase.NewCreateDaily(store, clock, nvim)
	createProject := kompusecase.NewCreateProject(store, repo, clock, nvim)
	createFree := kompusecase.NewCreateFree(store, nvim)
	captureDaily := kompusecase.NewCaptureDaily(store, clock)
	open := kompusecase.NewOpen(store, nvim)
	editNote := &kompusecase.EditNote{Store: store, Editor: nvim}

	return kompendiumcli.Deps{
		Store:  store,
		Rooter: nil, // apistore has no local filesystem root
		Repo:   repo,

		CreateDaily:   createDaily,
		CreateProject: createProject,
		CreateFree:    createFree,
		CaptureDaily:  captureDaily,
		Open:          open,

		ListNotes:       kompusecase.NewListNotes(store),
		SearchNotes:     kompusecase.NewSearchNotes(store, userID),
		RenderDaily:     kompusecase.NewRenderDaily(store),
		RenderBacklinks: kompusecase.NewRenderBacklinks(store, store), // apistore implements backlinkProvider

		DeleteNote: kompusecase.NewDeleteNote(store),
		EditNote:   editNote,

		EditCmd:   nvim.Cmd,
		IndexPath: "", // no local FTS5 index in server mode
	}, store, func() {}, nil
}

// parseEnvHoursDuration reads name as a positive float-of-hours and
// returns it as a Duration. Empty or malformed values return 0 so the
// downstream adapter can apply its own baseline. Centralised here so
// the worktime adapter doesn't have to know the env-var name.
func parseEnvHoursDuration(name string) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return 0
	}
	h, err := strconv.ParseFloat(v, 64)
	if err != nil || h <= 0 {
		return 0
	}
	return time.Duration(h * float64(time.Hour))
}

var rootCmd = &cobra.Command{
	Use:           "flow",
	Short:         "Workspace TUI sidekick",
	SilenceUsage:  true,
	SilenceErrors: true,
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
	xdgDataHome := os.Getenv("XDG_DATA_HOME")
	if xdgDataHome == "" {
		xdgDataHome = filepath.Join(home, ".local", "share")
	}
	indexPath := filepath.Join(xdgDataHome, "kompendium", "index.db")

	xdgStateHome := os.Getenv("XDG_STATE_HOME")
	if xdgStateHome == "" {
		xdgStateHome = filepath.Join(home, ".local", "state")
	}
	logClose := setupLogging(
		filepath.Join(xdgStateHome, "flow"),
		os.Getenv("FLOW_LOG_LEVEL"),
	)
	defer logClose()

	env := Env{
		WorktimeTargetHours: parseEnvHoursDuration("WORKTIME_TARGET_HOURS"),
		WorktimeLand:        os.Getenv("WORKTIME_LAND"),
		ServerURL:           os.Getenv("FLOW_SERVER_URL"),
		OIDCClientID:        envOrDefault("FLOW_OIDC_CLIENT_ID", "flow-cli"),
	}

	// Signal-aware context for buildDeps (SSE events client needs ctx at Start time).
	// This context also covers long-running subcommands (status --watch, markdown
	// view, kompendium browse) — they shut down cleanly on Ctrl+C / SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, cleanup, err := buildDeps(ctx, Paths{
		Home:                home,
		WorktimeLog:         filepath.Join(tmuxDir, "worktime.log"),
		TmuxDir:             tmuxDir,
		CacheDir:            filepath.Join(home, ".cache", "flow"),
		PluginsDir:          filepath.Join(tmuxDir, "plugins"),
		StateDir:            filepath.Join(home, ".local", "state", "flow"),
		Cheatsheet:          filepath.Join(tmuxDir, "cheatsheet.md"),
		SourceCodeRoot:      sourceRoot,
		KompendiumNotebook:  notebookRoot,
		KompendiumIndexPath: indexPath,
	}, env)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer cleanup()

	rootCmd.AddCommand(newLoginCmd(), newLogoutCmd(), newWhoamiCmd())
	rootCmd.AddCommand(cli.NewSidekickCmd(deps.Sidekick))
	rootCmd.AddCommand(cli.NewWorktimeCmd(deps.Worktime))
	rootCmd.AddCommand(cli.NewCheatsheetCmd(deps.Cheatsheet))
	rootCmd.AddCommand(cli.NewPaletteCmd(deps.Palette))
	rootCmd.AddCommand(cli.NewProjectsCmd(deps.Projects))
	rootCmd.AddCommand(cli.NewRepoCmd(deps.Repo))
	rootCmd.AddCommand(cli.NewMarkdownCmd())
	rootCmd.AddCommand(kompendiumcli.NewRootCmd(deps.Kompendium))
	rootCmd.AddCommand(newDocsCmd(deps.Docs))

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
