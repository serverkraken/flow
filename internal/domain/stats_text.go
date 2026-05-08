// Stats — plain text rendering. Lifted from the cli/worktime.go branch
// so both the CLI verb (`flow worktime stats`) and the worktime TUI's
// menu-Stats-Action share one writer. Domain layer was the right home:
// it sits next to WriteBrief / WriteCSV / WriteJSON and keeps any UI
// shell from re-implementing the formatting.
//
// Format is verbatim what `flow worktime stats` produced before the
// move — column-aligned, German labels, optional Tags / Frei sections
// only when populated. Tests in cli/worktime_test.go pin this output
// shape.

package domain

import (
	"fmt"
	"io"
)

// WriteStats writes a column-aligned plain-text summary of st to w.
// expr is the range expression that produced st (e.g. "month",
// "2026", "today") — surfaced as the first row so a copy-paste of the
// brief carries its own scope label.
//
// Body, Tags and Frei are emitted in three helpers so the gocognit
// budget stays in check; each helper short-circuits on the first I/O
// error and propagates it back.
func WriteStats(w io.Writer, expr string, st Stats) error {
	if err := writeStatsBody(w, expr, st); err != nil {
		return err
	}
	if err := writeStatsTags(w, st); err != nil {
		return err
	}
	return writeStatsDaysOff(w, st)
}

func writeStatsBody(w io.Writer, expr string, st Stats) error {
	rows := []struct {
		label string
		value string
		emit  bool
	}{
		{"Range:    ", expr, true},
		{"Tage:     ", fmt.Sprintf("%d", st.Days), true},
		{"Werktage: ", fmt.Sprintf("%d", st.Workdays), true},
		{"Total:    ", FmtDuration(st.Total), true},
		{"Schnitt:  ", FmtDuration(st.Avg), true},
		{
			"Max:      ", fmt.Sprintf("%s  (%s)", FmtDuration(st.Max), st.MaxDate.Format("2006-01-02")),
			!st.MaxDate.IsZero(),
		},
		{
			"Min:      ", fmt.Sprintf("%s  (%s)", FmtDuration(st.Min), st.MinDate.Format("2006-01-02")),
			!st.MinDate.IsZero(),
		},
		{"Ziele:    ", fmt.Sprintf("%d / %d", st.Hits, st.Workdays), true},
		{"Streak:   ", fmt.Sprintf("%d (best %d)", st.Streak, st.BestStreak), true},
		{"Saldo:    ", FmtSignedDuration(st.Overtime), true},
	}
	for _, r := range rows {
		if !r.emit {
			continue
		}
		if _, err := fmt.Fprintf(w, "%s%s\n", r.label, r.value); err != nil {
			return err
		}
	}
	return nil
}

func writeStatsTags(w io.Writer, st Stats) error {
	tags := st.TopTags(0)
	if len(tags) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "Tags:"); err != nil {
		return err
	}
	for _, t := range tags {
		if _, err := fmt.Fprintf(w, "  %-16s %s\n", t.Tag, FmtDuration(t.Total)); err != nil {
			return err
		}
	}
	return nil
}

func writeStatsDaysOff(w io.Writer, st Stats) error {
	if len(st.DaysOff) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "Frei:"); err != nil {
		return err
	}
	byKind := map[Kind]int{}
	for _, d := range st.DaysOff {
		byKind[d.Kind]++
	}
	for _, k := range AllKinds {
		c := byKind[k]
		if c == 0 {
			continue
		}
		if _, err := fmt.Fprintf(w, "  %-10s %d\n", k.LabelDe(), c); err != nil {
			return err
		}
	}
	return nil
}
