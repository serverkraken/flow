package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/spf13/cobra"
)

// berlinLoc is the timezone all legacy worktime timestamps are interpreted in.
// Loaded once; falls back to UTC only if the tz database is unavailable.
var berlinLoc = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// parseDateTimeBerlin builds a time from a "2006-01-02" date and a "15:04"
// clock string, interpreted in Europe/Berlin.
func parseDateTimeBerlin(date, hhmm string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04", date+" "+hhmm, berlinLoc)
}

// logEntry is one parsed worktime.log row. Seconds is the legacy elapsed-seconds
// column; it is captured for a divergence warning but never used for time math.
type logEntry struct {
	Line        int
	Start, Stop time.Time
	Seconds     int
}

type dayOffEntry struct {
	Line      int
	Date      string
	Kind      domain.Kind
	Label     string
	TargetMin int
}

// parseDayOffLine parses "date<TAB>kind<TAB>label[<TAB>hours]". ok=false for
// blank and "#"-comment lines. The Kind may be KindHoliday; the caller skips
// holidays (the server refuses to store them).
func parseDayOffLine(lineNo int, raw string) (dayOffEntry, bool, error) {
	t := strings.TrimSpace(raw)
	if t == "" || strings.HasPrefix(t, "#") {
		return dayOffEntry{}, false, nil
	}
	f := strings.Split(raw, "\t")
	if len(f) < 3 {
		return dayOffEntry{}, false, fmt.Errorf("expected at least 3 tab-separated columns, got %d", len(f))
	}
	if _, err := time.Parse("2006-01-02", f[0]); err != nil {
		return dayOffEntry{}, false, fmt.Errorf("date: %w", err)
	}
	kind, ok := domain.ParseKind(f[1])
	if !ok {
		return dayOffEntry{}, false, fmt.Errorf("unknown kind %q", f[1])
	}
	e := dayOffEntry{Line: lineNo, Date: f[0], Kind: kind, Label: strings.TrimSpace(f[2])}
	if len(f) >= 4 && strings.TrimSpace(f[3]) != "" {
		hours, err := strconv.ParseFloat(strings.TrimSpace(f[3]), 64)
		if err != nil {
			return dayOffEntry{}, false, fmt.Errorf("hours: %w", err)
		}
		e.TargetMin = int(hours * 60)
	}
	return e, true, nil
}

// parseLogLine parses a tab-separated "date<TAB>start<TAB>end<TAB>seconds" row.
// ok=false marks a blank line (skip, no error); a non-nil error marks a
// malformed row.
func parseLogLine(lineNo int, raw string) (logEntry, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return logEntry{}, false, nil
	}
	f := strings.Split(raw, "\t")
	if len(f) != 4 {
		return logEntry{}, false, fmt.Errorf("expected 4 tab-separated columns, got %d", len(f))
	}
	start, err := parseDateTimeBerlin(f[0], f[1])
	if err != nil {
		return logEntry{}, false, fmt.Errorf("start: %w", err)
	}
	stop, err := parseDateTimeBerlin(f[0], f[2])
	if err != nil {
		return logEntry{}, false, fmt.Errorf("stop: %w", err)
	}
	secs, err := strconv.Atoi(strings.TrimSpace(f[3]))
	if err != nil {
		return logEntry{}, false, fmt.Errorf("seconds: %w", err)
	}
	return logEntry{Line: lineNo, Start: start, Stop: stop, Seconds: secs}, true, nil
}

type wtImportStats struct {
	Sessions, DayOffs, Skipped, Links, Failed, ProjectsCreated int
	Warnings, Failures                                          []string
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// runWorktimeImport reads ~/worktime's files from dir and imports them.
// Per-row failures are isolated and collected; the run continues.
func runWorktimeImport(ctx context.Context, c *apiclient.Client, dir, projectName string, dryRun bool) (wtImportStats, error) {
	var st wtImportStats
	if err := importSessions(ctx, c, dir, projectName, dryRun, &st); err != nil {
		return st, err
	}
	if err := importDayOffs(ctx, c, dir, dryRun, &st); err != nil {
		return st, err
	}
	if err := importLinks(dir, &st); err != nil {
		return st, err
	}
	return st, nil
}

// importLinks parses worktime-links.tsv but writes nothing: the day↔daily-doc
// relationship is already encoded by the daily/<date> path convention that
// `flow docs import` preserves.
func importLinks(dir string, st *wtImportStats) error {
	raw, err := os.ReadFile(filepath.Join(dir, "worktime-links.tsv"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read worktime-links.tsv: %w", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Split(strings.TrimSpace(line), "\t")
		if len(f) < 2 || f[0] == "" {
			continue
		}
		st.Links++
		st.Warnings = append(st.Warnings, fmt.Sprintf(
			"worktime-links.tsv: %s → %s (covered by daily/<date> convention, nicht importiert)", f[0], f[1]))
	}
	return nil
}

func importDayOffs(ctx context.Context, c *apiclient.Client, dir string, dryRun bool, st *wtImportStats) error {
	raw, err := os.ReadFile(filepath.Join(dir, "worktime-dayoffs.tsv"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read worktime-dayoffs.tsv: %w", err)
	}
	for i, line := range strings.Split(string(raw), "\n") {
		e, ok, perr := parseDayOffLine(i+1, line)
		if perr != nil {
			st.Failed++
			st.Failures = append(st.Failures, fmt.Sprintf("worktime-dayoffs.tsv:%d: %v", i+1, perr))
			continue
		}
		if !ok {
			continue
		}
		if e.Kind == domain.KindHoliday {
			st.Skipped++ // computed from Bundesland, never stored
			continue
		}
		if dryRun {
			st.DayOffs++
			continue
		}
		if derr := c.AddDayOffs(ctx, e.Date, e.Date, string(e.Kind), e.Label, e.TargetMin, false); derr != nil {
			st.Failed++
			st.Failures = append(st.Failures, fmt.Sprintf("worktime-dayoffs.tsv:%d: %v", e.Line, derr))
			continue
		}
		st.DayOffs++
	}
	return nil
}

func worktimeImportCmd() *cobra.Command {
	var dryRun bool
	var projectName string
	cmd := &cobra.Command{
		Use:   "import [dir]",
		Short: "Importiere Worktime-Daten aus einer alten flow-Installation (~/worktime)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args0OrDefault(args)
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			st, err := runWorktimeImport(cmd.Context(), c, dir, projectName, dryRun)
			if err != nil {
				return err
			}
			mode := ""
			if dryRun {
				mode = " (dry-run)"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"gebucht %d · freie Tage %d · übersprungen %d · Links %d · Projekt %q · Projekte angelegt %d · Fehler %d%s\n",
				st.Sessions, st.DayOffs, st.Skipped, st.Links, projectName, st.ProjectsCreated, st.Failed, mode)
			for _, wmsg := range st.Warnings {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  ⚠ "+wmsg)
			}
			for _, f := range st.Failures {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+f)
			}
			if st.Failed > 0 {
				return fmt.Errorf("%d Zeile(n) fehlgeschlagen", st.Failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "parse und plane den Import, ohne zu schreiben")
	cmd.Flags().StringVar(&projectName, "project", "Import", "Projekt, dem importierte Sessions zugeordnet werden")
	return cmd
}

// args0OrDefault returns the positional dir arg, or ~/worktime when omitted.
func args0OrDefault(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "worktime"
	}
	return filepath.Join(home, "worktime")
}

func importSessions(ctx context.Context, c *apiclient.Client, dir, projectName string, dryRun bool, st *wtImportStats) error {
	raw, err := os.ReadFile(filepath.Join(dir, "worktime.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no sessions to import
		}
		return fmt.Errorf("read worktime.log: %w", err)
	}
	pr := newProjectResolver(c, dryRun)
	projectID, err := pr.resolve(ctx, projectName)
	if err != nil {
		return fmt.Errorf("resolve project %q: %w", projectName, err)
	}
	defer func() { st.ProjectsCreated += pr.created }()

	for i, line := range strings.Split(string(raw), "\n") {
		e, ok, perr := parseLogLine(i+1, line)
		if perr != nil {
			st.Failed++
			st.Failures = append(st.Failures, fmt.Sprintf("worktime.log:%d: %v", i+1, perr))
			continue
		}
		if !ok {
			continue
		}
		if diff := absDur(time.Duration(e.Seconds)*time.Second - e.Stop.Sub(e.Start)); diff > 5*time.Minute {
			st.Warnings = append(st.Warnings, fmt.Sprintf(
				"worktime.log:%d: Sekunden %d ≠ Uhrzeit-Dauer %s (importiere Uhrzeit)",
				e.Line, e.Seconds, e.Stop.Sub(e.Start)))
		}
		if dryRun {
			st.Sessions++
			continue
		}
		if _, aerr := c.AddSession(ctx, projectID, e.Start, e.Stop, "", ""); aerr != nil {
			if apiclient.IsConflict(aerr) {
				st.Skipped++
				continue
			}
			st.Failed++
			st.Failures = append(st.Failures, fmt.Sprintf("worktime.log:%d: %v", e.Line, aerr))
			continue
		}
		st.Sessions++
	}
	return nil
}
