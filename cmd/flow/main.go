// Package main is the flow CLI entry point.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/adapter/cheatsheetfs"
	"github.com/serverkraken/flow/internal/adapter/dayoffstsv"
	"github.com/serverkraken/flow/internal/adapter/editor"
	"github.com/serverkraken/flow/internal/adapter/flockstate"
	"github.com/serverkraken/flow/internal/adapter/fspaletteentries"
	"github.com/serverkraken/flow/internal/adapter/fsprojects"
	"github.com/serverkraken/flow/internal/adapter/glamourrenderer"
	"github.com/serverkraken/flow/internal/adapter/iniconfig"
	"github.com/serverkraken/flow/internal/adapter/jsonflowstate"
	"github.com/serverkraken/flow/internal/adapter/jsonpalettestats"
	"github.com/serverkraken/flow/internal/adapter/linkstsv"
	"github.com/serverkraken/flow/internal/adapter/systemclock"
	"github.com/serverkraken/flow/internal/adapter/tmuxbridge"
	"github.com/serverkraken/flow/internal/adapter/tsvsessions"
	"github.com/serverkraken/flow/internal/frontend/cli"
	"github.com/serverkraken/flow/internal/frontend/tui/components/theme"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/cheatsheet"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/palette"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/projects"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/spf13/cobra"
)

// Paths bundles every filesystem location the dependency graph reads or writes.
// Tests rewire this against t.TempDir() so the whole graph runs in isolation.
type Paths struct {
	WorktimeLog    string
	TmuxDir        string
	CacheDir       string
	PluginsDir     string
	StateDir       string // ~/.local/state/flow — palette stats etc.
	Cheatsheet     string
	SourceCodeRoot string // $SOURCECODE_ROOT or ~/Sourcecode — project enumeration root.
}

// Deps is the wired dependency graph. F4.1+ extends it with palette /
// projects / cheatsheet deps as those waves land.
type Deps struct {
	Worktime cli.WorktimeDeps
	Sidekick cli.SidekickDeps
}

func buildDeps(p Paths) Deps {
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
	noteLauncher := editor.New(envOr("KOMPENDIUM_BIN", "kompendium"), envOr("FLOW_NOTE_VIEWER", "glow"))
	flowState := jsonflowstate.New(
		filepath.Join(p.CacheDir, "state.json"),
		filepath.Join(p.CacheDir, "next-screen"),
	)
	cheatsheetReader := cheatsheetfs.New(p.Cheatsheet)
	mdRenderer := glamourrenderer.New()
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
			Worktime: func(pal theme.Palette) tea.Model {
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
			},
		},
	}
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

	deps := buildDeps(Paths{
		WorktimeLog:    filepath.Join(tmuxDir, "worktime.log"),
		TmuxDir:        tmuxDir,
		CacheDir:       filepath.Join(home, ".cache", "flow"),
		PluginsDir:     filepath.Join(tmuxDir, "plugins"),
		StateDir:       filepath.Join(home, ".local", "state", "flow"),
		Cheatsheet:     filepath.Join(tmuxDir, "cheatsheet.md"),
		SourceCodeRoot: sourceRoot,
	})

	rootCmd.AddCommand(cli.NewSidekickCmd(deps.Sidekick))
	rootCmd.AddCommand(cli.NewWorktimeCmd(deps.Worktime))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
