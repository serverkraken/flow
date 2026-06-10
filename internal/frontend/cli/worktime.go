// Package cli wires the worktime cobra command tree against domain values
// and use-case actions. It is the F4.1 frontend layer: no adapter imports,
// no direct I/O — every side effect goes through ports.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/spf13/cobra"
)

// fprintf and fprintln wrap fmt.Fprint* and discard the byte count
// + error. Print failures to cmd.OutOrStdout / ErrOrStderr are not
// recoverable in a CLI; matches the legacy tmuxRefresh discard
// pattern.
func fprintf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func fprintln(w io.Writer, args ...any) {
	_, _ = fmt.Fprintln(w, args...)
}

// WorktimeDeps is the dependency bundle the worktime cobra subcommand
// tree consumes. Constructed by the composition root and threaded into
// every RunE through closures.
//
// Screen is a factory typed `func(tk.Palette) tea.Model` so the `today`
// verb can run flow's worktime TUI as a standalone bubbletea program
// without cli/worktime importing the screen package directly (depguard
// keeps frontend-cli decoupled from frontend/tui/screen/<name>).
//
// New in Task 14: ProjectID-aware start/stop path. When ResolveProject,
// StartActiveSession and StopActiveSession are non-nil the new sqlite
// path is active; otherwise start/stop fall back to the legacy TSV
// SessionWriter path.
type WorktimeDeps struct {
	Clock          ports.Clock
	Tmux           ports.Tmux
	SessionWriter  *usecase.SessionWriter
	StatusComposer *usecase.StatusComposer
	Reporter       *usecase.Reporter
	Stats          *usecase.StatsComputer
	DayOffWriter   *usecase.DayOffWriter
	DayOffStore    ports.DayOffStore
	Reader         *usecase.WorktimeReader
	Screen         func(tk.Palette) tea.Model

	// New-path fields (Task 14). Nil → legacy SessionWriter path is used.

	// UserID is the resolved local user's ID for all project/session operations.
	UserID string

	// ResolveProject implements the 4-step smart-default cascade:
	// explicit → PWD-basename slug → MRU → auto-create "Allgemein".
	ResolveProject func(userID, explicitID, pwd string) (domain.Project, error)

	// StartActiveSession writes an ActiveSession locally and queues a
	// server-start push. tag and note are persisted on the row so a later
	// Stop can carry them through even when no flags are passed at stop time.
	// Returns ErrActiveSessionExists when one is already running for
	// (userID, projectID).
	StartActiveSession func(userID, projectID, tag, note string) (domain.ActiveSession, error)

	// StopActiveSession closes the active session, creates a finished Session
	// row and queues a server-stop push.
	StopActiveSession func(userID, projectID, tag, note string) (domain.Session, error)

	// ListActiveSessions returns the currently running sessions for the user.
	// Stop/toggle/pause operate on THE active session — not on whatever
	// project the cwd happens to resolve to.
	ListActiveSessions func(userID string) ([]domain.ActiveSession, error)

	// Migrate is the TSV-to-SQLite migration use case.
	// Nil until the composition root wires it; the subcommand's RunE
	// surfaces a clear error when unset.
	Migrate *usecase.MigrateTSV

	// PauseMarker is the per-device pause flag (flockstate worktime.pause).
	// Never synced; pause = stop the active session + set this marker so
	// resume knows to restart.
	PauseMarker ports.PauseStore

	// CorrectActiveStart moves the running session's start time (new path).
	// Nil → legacy SessionWriter.CorrectStart path is used.
	CorrectActiveStart func(userID string, ts time.Time) error
}

// NewWorktimeCmd constructs the `flow worktime` subcommand tree.
func NewWorktimeCmd(deps WorktimeDeps) *cobra.Command {
	worktimeCmd := &cobra.Command{
		Use:          "worktime",
		Short:        "Worktime subcommands",
		SilenceUsage: true,
	}

	worktimeCmd.AddCommand(
		newStatusCmd(deps),
		newStartCmd(deps),
		newPauseCmd(deps),
		newResumeCmd(deps),
		newBriefCmd(deps),
		newStopCmd(deps),
		newToggleCmd(deps),
		newCorrectCmd(deps),
		newExportCmd(deps),
		newStatsCmd(deps),
		newTodayCmd(deps),
		newTagCmd(deps),
		newNoteCmd(deps),
		newDayOffCmd(deps),
		newMigrateTSVCmd(MigrateTSVDeps{
			MigrateTSV: deps.Migrate,
			UserID:     deps.UserID,
		}),
	)
	return worktimeCmd
}

// runWorktimeToday is the production handler. Package-level var so
// tests can swap in a no-op and verify the cobra wiring without
// launching a real Bubble Tea program against a (non-existent) TTY.
// Mirrors the kompendium-cli runBrowse pattern. Tests must NOT call
// t.Parallel — the var is process-global; concurrent swaps race.
var runWorktimeToday = func(ctx context.Context, deps WorktimeDeps) error {
	tk.Init()
	pal := tk.Load()
	prog := tea.NewProgram(deps.Screen(pal), tea.WithContext(ctx))
	_, err := prog.Run()
	return err
}

// newTodayCmd opens flow's worktime TUI as a standalone bubbletea
// program, defaulting to the Heute tab. Same screen flow's sidekick
// hosts at `prefix+a 3` (or `flow sidekick` → `w`); just spawned
// directly without the surrounding 5-tab chrome.
//
// Mirrors `flow kompendium browse`: a single TUI surface, no flags,
// no shell-out, q to quit. The dotfiles' worktime sidekick view
// shells this verb to peek at today's sessions in the same styled
// frame the rest of flow's TUI uses.
func newTodayCmd(deps WorktimeDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "today",
		Short:        "Worktime TUI öffnen (Heute-Tab; Tab/1-4 wechselt zu Woche/History/Frei)",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deps.Screen == nil {
				return errors.New("worktime screen factory not wired (composition-root bug)")
			}
			return runWorktimeToday(cmd.Context(), deps)
		},
	}
}

func newStatusCmd(deps WorktimeDeps) *cobra.Command {
	var watch bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print tmux status-right segment",
		Long: `Druckt das tmux status-right Segment.

  --watch    refresht alle 60s (für non-tmux Statusbars).`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if !watch {
				fprintln(out, deps.StatusComposer.Compose())
				return nil
			}
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			fprintln(out, deps.StatusComposer.Compose())
			ctx := cmd.Context()
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
					fprintln(out, deps.StatusComposer.Compose())
				}
			}
		},
	}
	cmd.Flags().BoolVar(&watch, "watch", false, "alle 60s neu drucken")
	return cmd
}

func newStartCmd(deps WorktimeDeps) *cobra.Command {
	var force bool
	var projectFlag, tagFlag, noteFlag string
	cmd := &cobra.Command{
		Use:   "start [zeit]",
		Short: "Start worktime session (jetzt, HH:MM, -Nm, -NhMMm)",
		Long: `Startet eine Session.

  --force      überschreibt eine bereits laufende Session (Default: Fehler).
  --project    Projekt-Slug oder UUID; sonst smart-default (PWD → MRU → Allgemein).
  --tag        Intent-Tag für die Session (wird beim Stop übernommen wenn angegeben).
  --note       Freitext-Notiz (Vorab-Notiz; beim Stop ergänzen oder überschreiben).`,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// New sqlite path: use when ResolveProject + StartActiveSession
			// are wired in the composition root (Task 14+).
			if deps.ResolveProject != nil && deps.StartActiveSession != nil {
				return runStartNew(cmd, deps, projectFlag, tagFlag, noteFlag)
			}
			// Legacy TSV path: used by existing tests and pre-migration installs.
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			ts, err := domain.ParseStartArg(arg, deps.Clock.Now())
			if err != nil {
				return err
			}
			if force {
				err = deps.SessionWriter.StartForce(ts)
			} else {
				err = deps.SessionWriter.Start(ts)
			}
			if errors.Is(err, domain.ErrAlreadyRunning) {
				// Idempotent for tmux bindings — pressing start while a
				// session is already running prints a hint but exits 0
				// instead of raising stderr noise the binding cannot
				// react to. Pass --force to overwrite.
				fprintln(cmd.ErrOrStderr(), "Worktime läuft bereits — `flow worktime stop` oder `--force`")
				return nil
			}
			if err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			fprintf(cmd.ErrOrStderr(), "Worktime läuft seit %s\n", ts.Format("15:04"))
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "läuft bereits → trotzdem überschreiben")
	cmd.Flags().StringVar(&projectFlag, "project", "", "Projekt-Slug oder UUID; sonst smart-default")
	cmd.Flags().StringVar(&tagFlag, "tag", "", "Intent-Tag (wird beim Stop übernommen wenn dort nicht überschrieben)")
	cmd.Flags().StringVar(&noteFlag, "note", "", "Vorab-Notiz (wird beim Stop übernommen wenn dort nicht überschrieben)")
	return cmd
}

// runStartNew handles the new sqlite-backed start path (Task 14).
// Resolves the project via the smart-default cascade, then records an
// ActiveSession locally and queues a server-start push. The legacy
// time-argument (HH:MM backdate) is not supported on the new path yet —
// ActiveSessions.Start always uses time.Now (consistent with the spec).
func runStartNew(cmd *cobra.Command, deps WorktimeDeps, projectFlag, tag, note string) error {
	pwd, _ := os.Getwd()
	pr, err := deps.ResolveProject(deps.UserID, projectFlag, pwd)
	if err != nil {
		return err
	}
	if _, err := deps.StartActiveSession(deps.UserID, pr.ID, tag, note); err != nil {
		if errors.Is(err, usecase.ErrActiveSessionExists) {
			// Idempotent: already running is a soft no-op like the legacy path.
			fprintf(cmd.ErrOrStderr(), "Session auf '%s' läuft bereits\n", pr.Name)
			return nil
		}
		return err
	}
	_ = deps.Tmux.RefreshClient()
	fprintf(cmd.ErrOrStderr(), "Worktime läuft seit %s auf '%s'\n", deps.Clock.Now().Format("15:04"), pr.Name)
	return nil
}

func newPauseCmd(deps WorktimeDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "pause",
		Short:        "Aktive Session pausieren (resume mit `start`/`toggle`)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deps.ListActiveSessions != nil && deps.StopActiveSession != nil && deps.PauseMarker != nil {
				return runPauseNew(cmd, deps)
			}
			s, err := deps.SessionWriter.Pause()
			if err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			if s.Elapsed > 0 {
				h := int(s.Elapsed.Hours())
				m := int(s.Elapsed.Minutes()) % 60
				fprintf(cmd.ErrOrStderr(), "Pausiert nach %dh %02dm — `flow worktime resume` setzt fort\n", h, m)
			}
			return nil
		},
	}
}

// runPauseNew: stop the running session (idempotent no-op when idle) and
// set the per-device pause marker so resume restarts on the same project (MRU).
func runPauseNew(cmd *cobra.Command, deps WorktimeDeps) error {
	list, err := deps.ListActiveSessions(deps.UserID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return nil // idempotent like the legacy path: nothing running is fine
	}
	target := list[0]
	for _, a := range list[1:] {
		if a.StartedAt.Before(target.StartedAt) {
			target = a
		}
	}
	sess, err := deps.StopActiveSession(deps.UserID, target.ProjectID, "", "")
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		// Race: another shell stopped between ListActiveSessions and Stop.
		// Still set the pause marker so resume restarts the same project.
		if err := deps.PauseMarker.SetPause(deps.Clock.Now()); err != nil {
			return err
		}
		_ = deps.Tmux.RefreshClient()
		fprintln(cmd.ErrOrStderr(), "Pausiert — Session war bereits gestoppt")
		return nil
	}
	if err != nil {
		return err
	}
	if err := deps.PauseMarker.SetPause(deps.Clock.Now()); err != nil {
		return err
	}
	_ = deps.Tmux.RefreshClient()
	h := int(sess.Elapsed.Hours())
	m := int(sess.Elapsed.Minutes()) % 60
	fprintf(cmd.ErrOrStderr(), "Pausiert nach %dh %02dm — `flow worktime resume` setzt fort\n", h, m)
	return nil
}

func newResumeCmd(deps WorktimeDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "resume",
		Short:        "Nach Pause weiterarbeiten",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deps.ListActiveSessions != nil && deps.StartActiveSession != nil &&
				deps.ResolveProject != nil && deps.PauseMarker != nil {
				return runResumeNew(cmd, deps)
			}
			// SessionWriter.Resume is idempotent — already-running just
			// clears the pause marker and returns nil. The legacy
			// ErrAlreadyRunning branch was dead and has been removed.
			if err := deps.SessionWriter.Resume(); err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			fprintln(cmd.ErrOrStderr(), "Resume — Worktime läuft weiter")
			return nil
		},
	}
}

// runResumeNew: clear the pause marker; when idle, restart on the MRU
// project (empty pwd skips the cwd step of the resolve cascade, so the
// paused project — last touched at its start — wins).
func runResumeNew(cmd *cobra.Command, deps WorktimeDeps) error {
	list, err := deps.ListActiveSessions(deps.UserID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		pr, rerr := deps.ResolveProject(deps.UserID, "", "")
		if rerr != nil {
			return rerr
		}
		if _, serr := deps.StartActiveSession(deps.UserID, pr.ID, "", ""); serr != nil &&
			!errors.Is(serr, usecase.ErrActiveSessionExists) {
			return serr
		}
	}
	if err := deps.PauseMarker.ClearPause(); err != nil {
		return err
	}
	_ = deps.Tmux.RefreshClient()
	fprintln(cmd.ErrOrStderr(), "Resume — Worktime läuft weiter")
	return nil
}

func newBriefCmd(deps WorktimeDeps) *cobra.Command {
	var scopeFlag string
	cmd := &cobra.Command{
		Use:   "brief [week|month|YYYY-MM-DD]",
		Short: "Markdown-Briefing der Woche/des Monats nach stdout",
		Long: `Erzeugt einen Stand-up-tauglichen Markdown-Bericht.

Beispiele:
  flow worktime brief                  # aktuelle Woche
  flow worktime brief week
  flow worktime brief month
  flow worktime brief 2026-04-15       # Woche, in der das Datum liegt
  flow worktime brief --scope month 2026-04-15`,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := deps.Clock.Now()
			scope := domain.ReportWeek
			scopeFromFlag := false
			if cmd.Flags().Changed("scope") {
				scopeFromFlag = true
				if scopeFlag == "month" {
					scope = domain.ReportMonth
				}
			}
			if len(args) > 0 {
				arg := args[0]
				switch arg {
				case "week", "":
					if scopeFromFlag && scope != domain.ReportWeek {
						return fmt.Errorf("widersprüchliche scopes: --scope=%s und Argument %q", scopeFlag, arg)
					}
					scope = domain.ReportWeek
				case "month":
					if scopeFromFlag && scope != domain.ReportMonth {
						return fmt.Errorf("widersprüchliche scopes: --scope=%s und Argument %q", scopeFlag, arg)
					}
					scope = domain.ReportMonth
				default:
					if t, err := time.ParseInLocation("2006-01-02", arg, time.Local); err == nil {
						ref = t
					} else {
						return fmt.Errorf("unbekannter scope: %s (week|month|YYYY-MM-DD)", arg)
					}
				}
			}
			return deps.Reporter.WriteBrief(cmd.OutOrStdout(), ref, scope)
		},
	}
	cmd.Flags().StringVar(&scopeFlag, "scope", "week", "Bereich: week|month")
	return cmd
}

func newStopCmd(deps WorktimeDeps) *cobra.Command {
	var projectFlag, tagFlag, noteFlag string
	cmd := &cobra.Command{
		Use:   "stop [HH:MM]",
		Short: "Stop current worktime session (optional: custom stop time)",
		Long: `Stoppt die aktive Session.

  --project    Projekt-Slug oder UUID; sonst smart-default (PWD → MRU → Allgemein).
  --tag        Session-Tag (z.B. "deep", "review").
  --note       Freitext-Notiz zur Session.`,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// New sqlite path: use when ResolveProject + StopActiveSession +
			// ListActiveSessions are wired in the composition root (Task 14+).
			if deps.ResolveProject != nil && deps.StopActiveSession != nil && deps.ListActiveSessions != nil {
				return runStopNew(cmd, deps, projectFlag, tagFlag, noteFlag)
			}
			// Legacy TSV path.
			var s domain.Session
			var err error
			if len(args) > 0 {
				ts, parseErr := domain.ParseStartArg(args[0], deps.Clock.Now())
				if parseErr != nil {
					return parseErr
				}
				s, err = deps.SessionWriter.StopAt(ts)
			} else {
				s, err = deps.SessionWriter.Stop()
			}
			if errors.Is(err, domain.ErrNoActiveSession) {
				// Idempotent for the tmux binding (prefix+E): pressing stop
				// with nothing running is a no-op, not an error.
				return nil
			}
			if err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			h := int(s.Elapsed.Hours())
			m := int(s.Elapsed.Minutes()) % 60
			fprintf(cmd.ErrOrStderr(), "Gestoppt nach %dh %02dm\n", h, m)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectFlag, "project", "", "Projekt-Slug oder UUID; sonst smart-default")
	cmd.Flags().StringVar(&tagFlag, "tag", "", "Session-Tag (z.B. deep, review)")
	cmd.Flags().StringVar(&noteFlag, "note", "", "Freitext-Notiz zur Session")
	return cmd
}

// runStopNew handles the new sqlite-backed stop path: it stops the user's
// running session. --project disambiguates when several run in parallel.
func runStopNew(cmd *cobra.Command, deps WorktimeDeps, projectFlag, tag, note string) error {
	list, err := deps.ListActiveSessions(deps.UserID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fprintln(cmd.ErrOrStderr(), "Keine laufende Session")
		return nil
	}
	var projectID string
	switch {
	case projectFlag != "":
		pwd, _ := os.Getwd()
		pr, rerr := deps.ResolveProject(deps.UserID, projectFlag, pwd)
		if rerr != nil {
			return rerr
		}
		projectID = pr.ID
	case len(list) == 1:
		projectID = list[0].ProjectID
	default:
		return fmt.Errorf("%d Sessions laufen parallel — mit --project wählen", len(list))
	}
	sess, err := deps.StopActiveSession(deps.UserID, projectID, tag, note)
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		fprintln(cmd.ErrOrStderr(), "Keine laufende Session für dieses Projekt")
		return nil
	}
	if err != nil {
		return err
	}
	_ = deps.Tmux.RefreshClient()
	h := int(sess.Elapsed.Hours())
	m := int(sess.Elapsed.Minutes()) % 60
	fprintf(cmd.ErrOrStderr(), "Gestoppt nach %dh %02dm\n", h, m)
	return nil
}

func newToggleCmd(deps WorktimeDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "toggle",
		Aliases:      []string{"s"},
		Short:        "Start wenn idle, stopp wenn läuft (alias: s)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deps.ListActiveSessions != nil && deps.StopActiveSession != nil &&
				deps.ResolveProject != nil && deps.StartActiveSession != nil {
				return runToggleNew(cmd, deps)
			}
			// Legacy TSV/flockstate path (tests without new-path deps).
			msg, err := deps.SessionWriter.Toggle()
			if err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			fprintln(cmd.ErrOrStderr(), msg)
			return nil
		},
	}
}

// runToggleNew: stop the earliest running session when one exists, else
// resolve a project (pwd → MRU → Allgemein) and start.
func runToggleNew(cmd *cobra.Command, deps WorktimeDeps) error {
	list, err := deps.ListActiveSessions(deps.UserID)
	if err != nil {
		return err
	}
	if len(list) > 0 {
		target := list[0]
		for _, a := range list[1:] {
			if a.StartedAt.Before(target.StartedAt) {
				target = a
			}
		}
		sess, serr := deps.StopActiveSession(deps.UserID, target.ProjectID, "", "")
		if errors.Is(serr, ports.ErrActiveSessionNotFound) {
			// Race: ListActiveSessions saw the row but Stop didn't —
			// another device/shell stopped it between calls. Don't print
			// "Gestoppt nach 0h 00m" from a zero-value session.
			_ = deps.Tmux.RefreshClient()
			fprintln(cmd.ErrOrStderr(), "Keine laufende Session")
			return nil
		}
		if serr != nil {
			return serr
		}
		_ = deps.Tmux.RefreshClient()
		// Stop the earliest-StartedAt entry — that's the longest-running
		// session, the one the user most plausibly wants to end with a
		// keystroke. Multi-session disambiguation is `flow worktime stop --project`.
		fprintf(cmd.ErrOrStderr(), "Gestoppt nach %dh %02dm\n",
			int(sess.Elapsed.Hours()), int(sess.Elapsed.Minutes())%60)
		return nil
	}
	pwd, _ := os.Getwd()
	pr, err := deps.ResolveProject(deps.UserID, "", pwd)
	if err != nil {
		return err
	}
	if _, err := deps.StartActiveSession(deps.UserID, pr.ID, "", ""); err != nil {
		if errors.Is(err, usecase.ErrActiveSessionExists) {
			fprintf(cmd.ErrOrStderr(), "Session auf '%s' läuft bereits\n", pr.Name)
			return nil
		}
		return err
	}
	_ = deps.Tmux.RefreshClient()
	fprintf(cmd.ErrOrStderr(), "Worktime läuft seit %s auf '%s'\n",
		deps.Clock.Now().Format("15:04"), pr.Name)
	return nil
}

func newCorrectCmd(deps WorktimeDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "correct [HH:MM]",
		Short:        "Startzeit der laufenden Session korrigieren",
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			ts, err := domain.ParseStartArg(arg, deps.Clock.Now())
			if err != nil {
				return err
			}
			if deps.CorrectActiveStart != nil {
				if err := deps.CorrectActiveStart(deps.UserID, ts); err != nil {
					if errors.Is(err, ports.ErrActiveSessionNotFound) {
						fprintln(cmd.ErrOrStderr(), "Keine laufende Session")
						return nil
					}
					return err
				}
				_ = deps.Tmux.RefreshClient()
				fprintf(cmd.ErrOrStderr(), "Startzeit korrigiert auf %s\n", ts.Format("15:04"))
				return nil
			}
			// Legacy flockstate path.
			if err := deps.SessionWriter.CorrectStart(ts); err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			fprintf(cmd.ErrOrStderr(), "Startzeit korrigiert auf %s\n", ts.Format("15:04"))
			return nil
		},
	}
}

func newExportCmd(deps WorktimeDeps) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "export [range]",
		Short: "Sessions als CSV/JSON exportieren (range: today, week, month, YYYY, YYYY-MM, FROM..TO)",
		Long: `Exportiert Sessions in stdout.
Range-Beispiele:
  flow worktime export                 # alles
  flow worktime export today
  flow worktime export week
  flow worktime export month
  flow worktime export 2026
  flow worktime export 2026-04
  flow worktime export 2026-04-01..2026-04-30
Format: --format csv (default) | json`,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expr := ""
			if len(args) > 0 {
				expr = args[0]
			}
			r, err := domain.ParseRange(deps.Clock.Now(), expr)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch format {
			case "json":
				return deps.Reporter.WriteJSON(out, r)
			case "csv", "":
				return deps.Reporter.WriteCSV(out, r)
			default:
				return fmt.Errorf("unbekanntes Format: %s (csv|json)", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "csv", "Ausgabeformat: csv|json")
	return cmd
}

func newStatsCmd(deps WorktimeDeps) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "stats [range]",
		Short: "Aggregierte Statistiken (Total, Schnitt, Max, Min, Streak, Überzeit, Tags)",
		Long: `Aggregiert Sessions im angegebenen Range.
Range-Beispiele:
  flow worktime stats             # default month
  flow worktime stats today
  flow worktime stats week
  flow worktime stats 2026
  flow worktime stats 2026-04-01..2026-04-30
Format: --format text (default) | json`,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expr := "month"
			if len(args) > 0 {
				expr = args[0]
			}
			r, err := domain.ParseRange(deps.Clock.Now(), expr)
			if err != nil {
				return err
			}
			all, err := deps.Reader.History()
			if err != nil {
				return err
			}
			var records []domain.DayRecord
			var st domain.Stats
			if r.From.IsZero() && r.To.IsZero() {
				records = all
				// No clean range → fall back to the record-driven
				// Aggregate. Saldo here ignores any workdays without
				// records, but the user explicitly asked for "all".
				st = deps.Stats.Aggregate(records)
			} else {
				for _, d := range all {
					if r.ContainsDate(d.Date) {
						records = append(records, d)
					}
				}
				// AggregateRange so missed workdays inside the range
				// pull saldo down. domain.Range is half-open [From, To);
				// AggregateRange wants the same shape so r.To passes
				// through unchanged.
				st = deps.Stats.AggregateRange(records, r.From, r.To)
			}
			out := cmd.OutOrStdout()
			switch format {
			case "json":
				return printStatsJSON(out, expr, st)
			case "text", "":
				return domain.WriteStats(out, expr, st)
			default:
				return fmt.Errorf("unbekanntes Format: %s (text|json)", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "Ausgabeformat: text|json")
	return cmd
}

func newTagCmd(deps WorktimeDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "tag <session-idx> [tag]",
		Short:        "Tag der heutigen Session setzen (idx 1-basiert; leerer Tag löscht)",
		SilenceUsage: true,
		Args:         cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			idx, err := strconv.Atoi(args[0])
			if err != nil || idx < 1 {
				return fmt.Errorf("idx muss eine positive Zahl sein, war %q", args[0])
			}
			tag := ""
			if len(args) > 1 {
				tag = args[1]
			}
			if err := deps.SessionWriter.SetTag(deps.Clock.Now(), idx-1, tag); err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			return nil
		},
	}
}

func newNoteCmd(deps WorktimeDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "note <session-idx> [text]",
		Short:        "Notiz der heutigen Session setzen (idx 1-basiert; leerer Text löscht)",
		SilenceUsage: true,
		Args:         cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			idx, err := strconv.Atoi(args[0])
			if err != nil || idx < 1 {
				return fmt.Errorf("idx muss eine positive Zahl sein, war %q", args[0])
			}
			text := ""
			if len(args) > 1 {
				text = args[1]
			}
			if err := deps.SessionWriter.SetNote(deps.Clock.Now(), idx-1, text); err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			return nil
		},
	}
}

func newDayOffCmd(deps WorktimeDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "dayoff",
		Short:        "Feiertage/Urlaub/Krankheit verwalten",
		SilenceUsage: true,
	}
	cmd.AddCommand(
		newDayOffAddCmd(deps),
		newDayOffRemoveCmd(deps),
		newDayOffListCmd(deps),
		newDayOffSyncCmd(deps),
		newDayOffExportCmd(deps),
	)
	return cmd
}

func newDayOffAddCmd(deps WorktimeDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "add <date|FROM..TO> <kind> [label]",
		Short: "Tag(e) frei eintragen (kind: holiday|vacation|sick, alias: h|v|s)",
		Long: `Trägt einen freien Tag oder einen Range ein.
Beispiele:
  flow worktime dayoff add 2026-04-30 vacation "Brückentag"
  flow worktime dayoff add 2026-07-13..2026-07-26 vacation "Sommerurlaub"
  flow worktime dayoff add 2026-09-15 sick`,
		SilenceUsage: true,
		Args:         cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, ok := domain.ParseKind(args[1])
			if !ok {
				return fmt.Errorf("unbekannte kategorie: %q (holiday|vacation|sick)", args[1])
			}
			label := ""
			if len(args) >= 3 {
				label = args[2]
			}
			from, to, isRange, err := domain.ParseDateOrRange(args[0], time.Local)
			if err != nil {
				return err
			}
			errOut := cmd.ErrOrStderr()
			if isRange {
				n, err := deps.DayOffWriter.AddRange(from, to, kind, label)
				if err != nil {
					return err
				}
				_ = deps.Tmux.RefreshClient()
				fprintf(errOut, "%d Tag(e) als %s eingetragen\n", n, kind.LabelDe())
				return nil
			}
			if err := deps.DayOffWriter.Add(from, kind, label); err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			fprintf(errOut, "%s eingetragen für %s\n", kind.LabelDe(), from.Format("2006-01-02"))
			return nil
		},
	}
}

func newDayOffRemoveCmd(deps WorktimeDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "remove <date>",
		Aliases:      []string{"rm", "del"},
		Short:        "Eintrag für ein Datum entfernen",
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			d, err := time.ParseInLocation("2006-01-02", args[0], time.Local)
			if err != nil {
				return fmt.Errorf("ungültiges datum: %s (YYYY-MM-DD)", args[0])
			}
			if err := deps.DayOffWriter.Remove(d); err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			return nil
		},
	}
}

func newDayOffListCmd(deps WorktimeDeps) *cobra.Command {
	var year int
	cmd := &cobra.Command{
		Use:          "list",
		Aliases:      []string{"ls"},
		Short:        "Alle freien Tage anzeigen (default: aktuelles Jahr)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			y := year
			if y == 0 {
				y = deps.Clock.Now().Year()
			}
			from := time.Date(y, time.January, 1, 0, 0, 0, 0, time.Local)
			to := time.Date(y, time.December, 31, 0, 0, 0, 0, time.Local)
			entries := deps.DayOffStore.List(from, to)
			// Pure read verb → empty result is silent on stdout (no
			// rows). Surfacing 'keine Einträge für YEAR' on stderr was
			// confusing for `dayoff list --year 2099 | wc -l` which
			// expected 0 with no signal of a real problem; an empty
			// stdout is the standard Unix shape for empty results.
			sort.Slice(entries, func(i, j int) bool { return entries[i].Date.Before(entries[j].Date) })
			out := cmd.OutOrStdout()
			for _, d := range entries {
				fprintf(out, "%s  %-8s  %s\n", d.Date.Format("2006-01-02"), d.Kind.LabelDe(), d.Label)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&year, "year", 0, "Jahr (default: aktuelles)")
	return cmd
}

func newDayOffSyncCmd(deps WorktimeDeps) *cobra.Command {
	var land string
	var year int
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Bundesland-Feiertage automatisch eintragen (idempotent)",
		Long: `Befüllt worktime-dayoffs.tsv mit den gesetzlichen Feiertagen für das
gewählte Bundesland und Jahr. Vorhandene vacation/sick-Einträge bleiben
unangetastet.

  --land   BW BY BE BB HB HH HE MV NI NW RP SL SN ST SH TH (default NW)
           Aliase: NRW → NW, Bayern → BY
  --year   Default: aktuelles Jahr`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			y := year
			if y == 0 {
				y = deps.Clock.Now().Year()
			}
			l := land
			if l == "" {
				l = "NW"
			}
			added, skipped, err := deps.DayOffWriter.SyncGermanHolidays(y, l, time.Local)
			if err != nil {
				return err
			}
			if added > 0 {
				_ = deps.Tmux.RefreshClient()
			}
			fprintf(cmd.ErrOrStderr(), "%d Feiertag(e) hinzugefügt, %d übersprungen (%s/%d)\n",
				added, skipped, l, y)
			return nil
		},
	}
	cmd.Flags().StringVar(&land, "land", "NW", "Bundesland (NW, BY, BW, …)")
	cmd.Flags().IntVar(&year, "year", 0, "Jahr (default: aktuelles)")
	return cmd
}

func newDayOffExportCmd(deps WorktimeDeps) *cobra.Command {
	var year int
	var format string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Frei-Einträge exportieren (--format ics|tsv, default ics)",
		Long: `Exportiert Frei-Einträge in stdout.
  --format ics  RFC 5545 .ics Kalenderdatei (Default)
  --format tsv  rohes TSV (Date<TAB>Kind<TAB>Label)
  --year        Default: aktuelles Jahr`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			y := year
			if y == 0 {
				y = deps.Clock.Now().Year()
			}
			from := time.Date(y, time.January, 1, 0, 0, 0, 0, time.Local)
			to := time.Date(y, time.December, 31, 0, 0, 0, 0, time.Local)
			out := cmd.OutOrStdout()
			switch format {
			case "tsv":
				for _, d := range deps.DayOffStore.List(from, to) {
					fprintf(out, "%s\t%s\t%s\n",
						d.Date.Format("2006-01-02"), d.Kind, d.Label)
				}
				return nil
			case "ics", "":
				return deps.Reporter.WriteICS(out, from, to)
			default:
				return fmt.Errorf("unbekanntes Format: %s (ics|tsv)", format)
			}
		},
	}
	cmd.Flags().IntVar(&year, "year", 0, "Jahr (default: aktuelles)")
	cmd.Flags().StringVar(&format, "format", "ics", "Ausgabeformat: ics|tsv")
	return cmd
}

func printStatsJSON(w io.Writer, expr string, st domain.Stats) error {
	type tagRow struct {
		Tag            string `json:"tag"`
		ElapsedSeconds int64  `json:"elapsed_seconds"`
	}
	type dayOffRow struct {
		Date  string `json:"date"`
		Kind  string `json:"kind"`
		Label string `json:"label"`
	}
	tags := st.TopTags(0)
	tagsOut := make([]tagRow, 0, len(tags))
	for _, t := range tags {
		tagsOut = append(tagsOut, tagRow{Tag: t.Tag, ElapsedSeconds: int64(t.Total.Seconds())})
	}
	dOff := make([]dayOffRow, 0, len(st.DaysOff))
	for _, d := range st.DaysOff {
		dOff = append(dOff, dayOffRow{
			Date: d.Date.Format("2006-01-02"), Kind: string(d.Kind), Label: d.Label,
		})
	}
	out := map[string]any{
		"range":            expr,
		"days":             st.Days,
		"workdays":         st.Workdays,
		"total_seconds":    int64(st.Total.Seconds()),
		"avg_seconds":      int64(st.Avg.Seconds()),
		"max_seconds":      int64(st.Max.Seconds()),
		"min_seconds":      int64(st.Min.Seconds()),
		"max_date":         dateOrEmpty(st.MaxDate),
		"min_date":         dateOrEmpty(st.MinDate),
		"hits":             st.Hits,
		"streak":           st.Streak,
		"best_streak":      st.BestStreak,
		"overtime_seconds": int64(st.Overtime.Seconds()),
		"by_tag":           tagsOut,
		"days_off":         dOff,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func dateOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}
