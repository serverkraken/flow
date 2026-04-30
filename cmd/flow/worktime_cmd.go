package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
	"github.com/spf13/cobra"
)

func tmuxRefresh() { _ = exec.Command("tmux", "refresh-client", "-S").Run() }

var worktimeCmd = &cobra.Command{
	Use:          "worktime",
	Short:        "Worktime subcommands",
	SilenceUsage: true,
}

var wtStatusWatch bool

var wtStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print tmux status-right segment",
	Long: `Druckt das tmux status-right Segment.

  --watch    refresht alle 60s (für non-tmux Statusbars).`,
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		if !wtStatusWatch {
			fmt.Print(worktime.StatusSegment())
			return nil
		}
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		fmt.Println(worktime.StatusSegment())
		for range ticker.C {
			fmt.Println(worktime.StatusSegment())
		}
		return nil
	},
}

var wtStartForce bool

var wtStartCmd = &cobra.Command{
	Use:          "start [zeit]",
	Short:        "Start worktime session (jetzt, HH:MM, -Nm, -NhMMm)",
	Long: `Startet eine Session.

  --force    überschreibt eine bereits laufende Session (Default: Fehler).`,
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		arg := ""
		if len(args) > 0 {
			arg = args[0]
		}
		ts, err := worktime.ParseStartArg(arg)
		if err != nil {
			return err
		}
		startFn := worktime.Start
		if wtStartForce {
			startFn = worktime.StartForce
		}
		if err := startFn(ts); err != nil {
			if errors.Is(err, worktime.ErrAlreadyRunning) {
				return fmt.Errorf("eine Session läuft bereits — `flow worktime stop` oder `--force`")
			}
			return err
		}
		tmuxRefresh()
		fmt.Fprintf(os.Stderr, "Worktime läuft seit %s\n", ts.Format("15:04"))
		return nil
	},
}

var wtPauseCmd = &cobra.Command{
	Use:          "pause",
	Short:        "Aktive Session pausieren (resume mit `start`/`toggle`)",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		s, err := worktime.Pause()
		if err != nil {
			return err
		}
		tmuxRefresh()
		if s.Elapsed > 0 {
			h := int(s.Elapsed.Hours())
			m := int(s.Elapsed.Minutes()) % 60
			fmt.Fprintf(os.Stderr, "Pausiert nach %dh %02dm — `flow worktime resume` setzt fort\n", h, m)
		}
		return nil
	},
}

var wtResumeCmd = &cobra.Command{
	Use:          "resume",
	Short:        "Nach Pause weiterarbeiten",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		if err := worktime.Resume(); err != nil {
			if errors.Is(err, worktime.ErrAlreadyRunning) {
				return fmt.Errorf("läuft bereits")
			}
			return err
		}
		tmuxRefresh()
		fmt.Fprintln(os.Stderr, "Resume — Worktime läuft weiter")
		return nil
	},
}

var wtBriefScope string

var wtBriefCmd = &cobra.Command{
	Use:          "brief [week|month|YYYY-MM-DD]",
	Short:        "Markdown-Briefing der Woche/des Monats nach stdout",
	Long: `Erzeugt einen Stand-up-tauglichen Markdown-Bericht.

Beispiele:
  flow worktime brief                  # aktuelle Woche
  flow worktime brief week
  flow worktime brief month
  flow worktime brief 2026-04-15       # Woche, in der das Datum liegt
  flow worktime brief --scope month 2026-04-15`,
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ref := time.Now()
		scope := worktime.ReportWeek
		if wtBriefScope == "month" {
			scope = worktime.ReportMonth
		}
		if len(args) > 0 {
			arg := args[0]
			switch arg {
			case "week", "":
				scope = worktime.ReportWeek
			case "month":
				scope = worktime.ReportMonth
			default:
				if t, err := time.ParseInLocation("2006-01-02", arg, time.Local); err == nil {
					ref = t
				} else {
					return fmt.Errorf("unbekannter scope: %s (week|month|YYYY-MM-DD)", arg)
				}
			}
		}
		return worktime.WriteMarkdownBrief(os.Stdout, ref, scope)
	},
}

var (
	wtDayOffSyncLand string
	wtDayOffSyncYear int
)

var wtDayOffSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Bundesland-Feiertage automatisch eintragen (idempotent)",
	Long: `Befüllt worktime-dayoffs.tsv mit den gesetzlichen Feiertagen für das
gewählte Bundesland und Jahr. Vorhandene vacation/sick-Einträge bleiben
unangetastet.

  --land   BW BY BE BB HB HH HE MV NI NW RP SL SN ST SH TH (default NW)
           Aliase: NRW → NW, Bayern → BY
  --year   Default: aktuelles Jahr`,
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		year := wtDayOffSyncYear
		if year == 0 {
			year = time.Now().Year()
		}
		land := wtDayOffSyncLand
		if land == "" {
			land = "NW"
		}
		added, skipped, err := worktime.SyncGermanHolidays(year, land)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "%d Feiertag(e) hinzugefügt, %d übersprungen (%s/%d)\n",
			added, skipped, land, year)
		return nil
	},
}

var wtStopCmd = &cobra.Command{
	Use:          "stop [HH:MM]",
	Short:        "Stop current worktime session (optional: custom stop time)",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		var s worktime.Session
		var err error
		if len(args) > 0 {
			ts, parseErr := worktime.ParseStartArg(args[0])
			if parseErr != nil {
				return parseErr
			}
			s, err = worktime.StopAt(ts)
		} else {
			s, err = worktime.Stop()
		}
		if errors.Is(err, worktime.ErrNoActiveSession) {
			// Idempotent for the tmux binding (prefix+E): pressing stop with
			// nothing running is a no-op, not an error.
			return nil
		}
		if err != nil {
			return err
		}
		tmuxRefresh()
		h := int(s.Elapsed.Hours())
		m := int(s.Elapsed.Minutes()) % 60
		fmt.Fprintf(os.Stderr, "Gestoppt nach %dh %02dm\n", h, m)
		return nil
	},
}

var wtToggleCmd = &cobra.Command{
	Use:          "toggle",
	Aliases:      []string{"s"},
	Short:        "Start wenn idle, stopp wenn läuft (alias: s)",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		msg, err := worktime.Toggle()
		if err != nil {
			return err
		}
		tmuxRefresh()
		fmt.Fprintln(os.Stderr, msg)
		return nil
	},
}

var wtCorrectCmd = &cobra.Command{
	Use:          "correct [HH:MM]",
	Short:        "Startzeit der laufenden Session korrigieren",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		arg := ""
		if len(args) > 0 {
			arg = args[0]
		}
		ts, err := worktime.ParseStartArg(arg)
		if err != nil {
			return err
		}
		if err := worktime.CorrectStart(ts); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Startzeit korrigiert auf %s\n", ts.Format("15:04"))
		return nil
	},
}

var wtExportFormat string

var wtExportCmd = &cobra.Command{
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
	RunE: func(_ *cobra.Command, args []string) error {
		expr := ""
		if len(args) > 0 {
			expr = args[0]
		}
		r, err := worktime.ParseRange(time.Now(), expr)
		if err != nil {
			return err
		}
		switch wtExportFormat {
		case "json":
			return worktime.ExportJSON(os.Stdout, r)
		case "csv", "":
			return worktime.ExportCSV(os.Stdout, r)
		default:
			return fmt.Errorf("unbekanntes Format: %s (csv|json)", wtExportFormat)
		}
	},
}

var wtStatsFormat string

var wtStatsCmd = &cobra.Command{
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
	RunE: func(_ *cobra.Command, args []string) error {
		expr := "month"
		if len(args) > 0 {
			expr = args[0]
		}
		r, err := worktime.ParseRange(time.Now(), expr)
		if err != nil {
			return err
		}
		records, err := loadDayRecordsInRange(r)
		if err != nil {
			return err
		}
		st := worktime.Aggregate(records)

		switch wtStatsFormat {
		case "json":
			return printStatsJSON(expr, st)
		case "text", "":
			printStatsText(expr, st)
			return nil
		default:
			return fmt.Errorf("unbekanntes Format: %s (text|json)", wtStatsFormat)
		}
	},
}

func printStatsText(expr string, st worktime.Stats) {
	fmt.Printf("Range:    %s\n", expr)
	fmt.Printf("Tage:     %d\n", st.Days)
	fmt.Printf("Werktage: %d\n", st.Workdays)
	fmt.Printf("Total:    %s\n", fmtDur(st.Total))
	fmt.Printf("Schnitt:  %s\n", fmtDur(st.Avg))
	if !st.MaxDate.IsZero() {
		fmt.Printf("Max:      %s  (%s)\n", fmtDur(st.Max), st.MaxDate.Format("2006-01-02"))
	}
	if !st.MinDate.IsZero() {
		fmt.Printf("Min:      %s  (%s)\n", fmtDur(st.Min), st.MinDate.Format("2006-01-02"))
	}
	fmt.Printf("Ziele:    %d / %d\n", st.Hits, st.Workdays)
	fmt.Printf("Streak:   %d (best %d)\n", st.Streak, st.BestStreak)
	fmt.Printf("Saldo:    %s\n", fmtSignedDur(st.Overtime))
	if tags := st.TopTags(0); len(tags) > 0 {
		fmt.Println("Tags:")
		for _, t := range tags {
			fmt.Printf("  %-16s %s\n", t.Tag, fmtDur(t.Total))
		}
	}
	if len(st.DaysOff) > 0 {
		fmt.Println("Frei:")
		byKind := map[worktime.Kind]int{}
		for _, d := range st.DaysOff {
			byKind[d.Kind]++
		}
		for _, k := range worktime.AllKinds {
			if c := byKind[k]; c > 0 {
				fmt.Printf("  %-10s %d\n", k.LabelDe(), c)
			}
		}
	}
}

func printStatsJSON(expr string, st worktime.Stats) error {
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
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func dateOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

var wtTagCmd = &cobra.Command{
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
		return worktime.SetTag(time.Now(), idx-1, tag)
	},
}

var wtNoteCmd = &cobra.Command{
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
		return worktime.SetNote(time.Now(), idx-1, text)
	},
}

// — dayoff subcommands —

var wtDayOffCmd = &cobra.Command{
	Use:          "dayoff",
	Short:        "Feiertage/Urlaub/Krankheit verwalten",
	SilenceUsage: true,
}

var wtDayOffAddCmd = &cobra.Command{
	Use:   "add <date|FROM..TO> <kind> [label]",
	Short: "Tag(e) frei eintragen (kind: holiday|vacation|sick, alias: h|v|s)",
	Long: `Trägt einen freien Tag oder einen Range ein.
Beispiele:
  flow worktime dayoff add 2026-04-30 vacation "Brückentag"
  flow worktime dayoff add 2026-07-13..2026-07-26 vacation "Sommerurlaub"
  flow worktime dayoff add 2026-09-15 sick`,
	SilenceUsage: true,
	Args:         cobra.RangeArgs(2, 3),
	RunE: func(_ *cobra.Command, args []string) error {
		kind, ok := worktime.ParseKind(args[1])
		if !ok {
			return fmt.Errorf("unbekannte kategorie: %q (holiday|vacation|sick)", args[1])
		}
		label := ""
		if len(args) >= 3 {
			label = args[2]
		}
		from, to, isRange, err := parseDateOrRange(args[0])
		if err != nil {
			return err
		}
		if isRange {
			n, err := worktime.AddDayOffRange(from, to, kind, label)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "%d Tag(e) als %s eingetragen\n", n, kind.LabelDe())
			return nil
		}
		if err := worktime.AddDayOff(from, kind, label); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "%s eingetragen für %s\n", kind.LabelDe(), from.Format("2006-01-02"))
		return nil
	},
}

var wtDayOffRemoveCmd = &cobra.Command{
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
		return worktime.RemoveDayOff(d)
	},
}

var wtDayOffListYear int

var wtDayOffListCmd = &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "Alle freien Tage anzeigen (default: aktuelles Jahr)",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		year := wtDayOffListYear
		if year == 0 {
			year = time.Now().Year()
		}
		from := time.Date(year, time.January, 1, 0, 0, 0, 0, time.Local)
		to := time.Date(year, time.December, 31, 0, 0, 0, 0, time.Local)
		entries := worktime.ListDayOffs(from, to)
		if len(entries) == 0 {
			fmt.Fprintf(os.Stderr, "keine Einträge für %d\n", year)
			return nil
		}
		// Stable order is ListDayOffs's responsibility, but a defensive sort
		// here makes the output independent of internal ordering changes.
		sort.Slice(entries, func(i, j int) bool { return entries[i].Date.Before(entries[j].Date) })
		for _, d := range entries {
			fmt.Printf("%s  %-8s  %s\n", d.Date.Format("2006-01-02"), d.Kind.LabelDe(), d.Label)
		}
		return nil
	},
}

// parseDateOrRange handles "YYYY-MM-DD" or "YYYY-MM-DD..YYYY-MM-DD".
func parseDateOrRange(s string) (from, to time.Time, isRange bool, err error) {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '.' && s[i+1] == '.' {
			fromStr := s[:i]
			toStr := s[i+2:]
			f, e1 := time.ParseInLocation("2006-01-02", fromStr, time.Local)
			t, e2 := time.ParseInLocation("2006-01-02", toStr, time.Local)
			if e1 != nil {
				return time.Time{}, time.Time{}, false, fmt.Errorf("from: %w", e1)
			}
			if e2 != nil {
				return time.Time{}, time.Time{}, false, fmt.Errorf("to: %w", e2)
			}
			return f, t, true, nil
		}
	}
	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("ungültiges datum: %s (YYYY-MM-DD oder YYYY-MM-DD..YYYY-MM-DD)", s)
	}
	return d, d, false, nil
}

func init() {
	wtExportCmd.Flags().StringVar(&wtExportFormat, "format", "csv", "Ausgabeformat: csv|json")
	wtStatsCmd.Flags().StringVar(&wtStatsFormat, "format", "text", "Ausgabeformat: text|json")
	wtStatusCmd.Flags().BoolVar(&wtStatusWatch, "watch", false, "alle 60s neu drucken")
	wtStartCmd.Flags().BoolVar(&wtStartForce, "force", false, "läuft bereits → trotzdem überschreiben")
	wtBriefCmd.Flags().StringVar(&wtBriefScope, "scope", "week", "Bereich: week|month")
	wtDayOffListCmd.Flags().IntVar(&wtDayOffListYear, "year", 0, "Jahr (default: aktuelles)")
	wtDayOffSyncCmd.Flags().StringVar(&wtDayOffSyncLand, "land", "NW", "Bundesland (NW, BY, BW, …)")
	wtDayOffSyncCmd.Flags().IntVar(&wtDayOffSyncYear, "year", 0, "Jahr (default: aktuelles)")
	wtDayOffCmd.AddCommand(wtDayOffAddCmd, wtDayOffRemoveCmd, wtDayOffListCmd, wtDayOffSyncCmd)
}

func loadDayRecordsInRange(r worktime.Range) ([]worktime.DayRecord, error) {
	all, err := worktime.LoadHistory()
	if err != nil {
		return nil, err
	}
	if r.From.IsZero() && r.To.IsZero() {
		return all, nil
	}
	out := make([]worktime.DayRecord, 0, len(all))
	for _, d := range all {
		if r.ContainsDate(d.Date) {
			out = append(out, d)
		}
	}
	return out, nil
}

func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

func fmtSignedDur(d time.Duration) string {
	sign := "+"
	if d < 0 {
		sign = "-"
		d = -d
	}
	return fmt.Sprintf("%s%dh %02dm", sign, int(d.Hours()), int(d.Minutes())%60)
}
