package worktime

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// ReportRange is the time range a Markdown brief covers.
type ReportRange int

const (
	ReportWeek  ReportRange = 0
	ReportMonth ReportRange = 1
)

// WriteMarkdownBrief writes a Stand-up-ready Markdown summary of the given
// range to w. Used by `flow worktime brief week|month` and the History
// "yank as markdown" action.
//
// Layout:
//
//	# Worktime · KW NN · DD.MM. – DD.MM.YYYY
//	## Übersicht (total, target, saldo, streak, hits)
//	## Tage     (per-day with sessions inline)
//	## Tags     (tag → total, sorted desc)
//	## Frei     (day-offs in range, if any)
//
// The format is plain Markdown so it pastes cleanly into Slack/Notion/Linear.
func WriteMarkdownBrief(w io.Writer, ref time.Time, scope ReportRange) error {
	from, to, title := briefBounds(ref, scope)

	hist, err := LoadHistory()
	if err != nil {
		return err
	}
	records := filterRecords(hist, from, to)
	st := Aggregate(records)

	fmt.Fprintf(w, "# %s\n\n", title)

	// Overview.
	fmt.Fprintf(w, "## Übersicht\n")
	fmt.Fprintf(w, "- **Total**: %s / %s  (%s)\n",
		fmtDur(st.Total), fmtDur(plannedTarget(from, to)), fmtSigned(st.Overtime))
	fmt.Fprintf(w, "- **Werktage**: %d  ·  **Ziele**: %d / %d  ·  **Streak**: %d (best %d)\n",
		st.Workdays, st.Hits, st.Workdays, st.Streak, st.BestStreak)
	if st.Days > 0 {
		fmt.Fprintf(w, "- **Schnitt**: %s\n", fmtDur(st.Avg))
	}
	fmt.Fprintln(w)

	// Per-day list.
	if len(records) > 0 {
		fmt.Fprintf(w, "## Tage\n")
		// Records are newest-first from LoadHistory; reverse for chronological brief.
		for i := len(records) - 1; i >= 0; i-- {
			r := records[i]
			tick := ""
			if r.Total >= r.Target && r.Target > 0 {
				tick = "  ✓"
			}
			fmt.Fprintf(w, "- **%s, %s** — %s / %s%s\n",
				weekdayShortDe(r.Date.Weekday()),
				r.Date.Format("02.01."),
				fmtDur(r.Total),
				fmtDur(r.Target),
				tick,
			)
			for _, s := range r.Sessions {
				tagBit := ""
				if s.Tag != "" {
					tagBit = "  [" + s.Tag + "]"
				}
				noteBit := ""
				if s.Note != "" {
					noteBit = "  — " + s.Note
				}
				fmt.Fprintf(w, "  - %s–%s  (%s)%s%s\n",
					s.Start.Format("15:04"),
					s.Stop.Format("15:04"),
					fmtDur(s.Elapsed),
					tagBit,
					noteBit,
				)
			}
		}
		fmt.Fprintln(w)
	}

	// Tags.
	if tags := st.TopTags(0); len(tags) > 0 {
		fmt.Fprintf(w, "## Tags\n")
		for _, t := range tags {
			fmt.Fprintf(w, "- **%s** — %s\n", t.Tag, fmtDur(t.Total))
		}
		if st.Untagged > 0 {
			fmt.Fprintf(w, "- _(ohne Tag)_ — %s\n", fmtDur(st.Untagged))
		}
		fmt.Fprintln(w)
	}

	// Day-offs.
	offs := ListDayOffs(from, to.AddDate(0, 0, -1))
	if len(offs) > 0 {
		fmt.Fprintf(w, "## Frei\n")
		for _, d := range offs {
			fmt.Fprintf(w, "- **%s, %s** — %s%s\n",
				weekdayShortDe(d.Date.Weekday()),
				d.Date.Format("02.01."),
				d.Kind.LabelDe(),
				labelSuffix(d.Label),
			)
		}
		fmt.Fprintln(w)
	}
	return nil
}

func labelSuffix(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "  ·  " + s
}

// briefBounds resolves the [from, to) span and the title for a brief.
func briefBounds(ref time.Time, scope ReportRange) (from, to time.Time, title string) {
	switch scope {
	case ReportMonth:
		from = time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, ref.Location())
		to = from.AddDate(0, 1, 0)
		title = fmt.Sprintf("Worktime · %s %d", monthShortDe(ref.Month()), ref.Year())
	default:
		mon := isoMonday(ref)
		_, wn := mon.ISOWeek()
		from = mon
		to = mon.AddDate(0, 0, 7)
		sun := mon.AddDate(0, 0, 6)
		title = fmt.Sprintf("Worktime · KW %d · %02d.%02d. – %02d.%02d.%d",
			wn, mon.Day(), mon.Month(), sun.Day(), sun.Month(), sun.Year())
	}
	return
}

// plannedTarget sums TargetFor over all workdays in [from, to). Day-offs
// reduce the planned target — that's how the saldo line stays meaningful in
// vacation weeks.
func plannedTarget(from, to time.Time) time.Duration {
	var sum time.Duration
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		if !IsWorkday(d) {
			continue
		}
		sum += TargetFor(d)
	}
	return sum
}

func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

func fmtSigned(d time.Duration) string {
	sign := "+"
	if d < 0 {
		sign = "-"
		d = -d
	}
	return fmt.Sprintf("%s%dh %02dm", sign, int(d.Hours()), int(d.Minutes())%60)
}

var (
	weekdayShortDeMap = [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}
	monthShortDeMap   = [13]string{"", "Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}
)

func weekdayShortDe(wd time.Weekday) string { return weekdayShortDeMap[wd] }
func monthShortDe(m time.Month) string      { return monthShortDeMap[m] }
