package domain

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// BriefInputs is everything WriteBrief needs to render a Markdown summary.
// All data is supplied by the caller — the function is I/O-free, no access
// to LoadHistory / ListDayOffs / TargetFor inside.
type BriefInputs struct {
	Title   string        // header line, typically built by BriefBounds
	Records []DayRecord   // pre-filtered to the brief's date range
	Stats   Stats         // typically Aggregate(Records, …)
	Planned time.Duration // PlannedTarget over the same range
	DayOffs []DayOff      // day-offs that fall inside the range, sorted by date
}

// WriteBrief writes a Stand-up-ready Markdown summary to w.
//
// Layout:
//
//	# {Title}
//	## Übersicht  (total / planned / saldo, workdays, hits, streak, schnitt)
//	## Tage       (newest-last per-day with sessions inline)
//	## Tags       (tag → total, sorted desc)
//	## Frei       (day-offs in range, if any)
//
// The body is composed in a strings.Builder and flushed once at the end —
// keeps the per-line writes cheap and consolidates the only failure point
// to the final io.WriteString.
func WriteBrief(w io.Writer, b BriefInputs) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", b.Title)
	briefOverview(&sb, b)
	briefDays(&sb, b.Records)
	briefTags(&sb, b.Stats)
	briefDayOffs(&sb, b.DayOffs)

	_, err := io.WriteString(w, sb.String())
	return err
}

func briefOverview(sb *strings.Builder, b BriefInputs) {
	fmt.Fprintln(sb, "## Übersicht")
	fmt.Fprintf(sb, "- **Total**: %s / %s  (%s)\n",
		FmtDuration(b.Stats.Total), FmtDuration(b.Planned), FmtSignedDuration(b.Stats.Overtime))
	fmt.Fprintf(sb, "- **Werktage**: %d  ·  **Ziele**: %d / %d  ·  **Streak**: %d (best %d)\n",
		b.Stats.Workdays, b.Stats.Hits, b.Stats.Workdays, b.Stats.Streak, b.Stats.BestStreak)
	if b.Stats.Days > 0 {
		fmt.Fprintf(sb, "- **Schnitt**: %s\n", FmtDuration(b.Stats.Avg))
	}
	fmt.Fprintln(sb)
}

func briefDays(sb *strings.Builder, records []DayRecord) {
	if len(records) == 0 {
		return
	}
	fmt.Fprintln(sb, "## Tage")
	// Records arrive newest-first from LoadHistory; reverse for chronological brief.
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		tick := ""
		if r.Total >= r.Target && r.Target > 0 {
			tick = "  ✓"
		}
		fmt.Fprintf(
			sb, "- **%s, %s** — %s / %s%s\n",
			WeekdayShortDe(r.Date.Weekday()),
			r.Date.Format("02.01."),
			FmtDuration(r.Total),
			FmtDuration(r.Target),
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
			fmt.Fprintf(
				sb, "  - %s–%s  (%s)%s%s\n",
				s.Start.Format("15:04"),
				s.Stop.Format("15:04"),
				FmtDuration(s.Elapsed),
				tagBit,
				noteBit,
			)
		}
	}
	fmt.Fprintln(sb)
}

func briefTags(sb *strings.Builder, stats Stats) {
	tags := stats.TopTags(0)
	if len(tags) == 0 {
		return
	}
	fmt.Fprintln(sb, "## Tags")
	for _, t := range tags {
		fmt.Fprintf(sb, "- **%s** — %s\n", t.Tag, FmtDuration(t.Total))
	}
	if stats.Untagged > 0 {
		fmt.Fprintf(sb, "- _(ohne Tag)_ — %s\n", FmtDuration(stats.Untagged))
	}
	fmt.Fprintln(sb)
}

func briefDayOffs(sb *strings.Builder, dayoffs []DayOff) {
	if len(dayoffs) == 0 {
		return
	}
	fmt.Fprintln(sb, "## Frei")
	for _, d := range dayoffs {
		fmt.Fprintf(
			sb, "- **%s, %s** — %s%s\n",
			WeekdayShortDe(d.Date.Weekday()),
			d.Date.Format("02.01."),
			d.Kind.LabelDe(),
			briefLabelSuffix(d.Label),
		)
	}
	fmt.Fprintln(sb)
}

// briefLabelSuffix prefixes the label with "  ·  " when non-empty so the
// "Frei" rows render cleanly with or without an annotation.
func briefLabelSuffix(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "  ·  " + s
}
