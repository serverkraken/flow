package domain

import (
	"fmt"
	"sort"
	"time"
)

// ReportRange is the time range a Markdown brief covers.
type ReportRange int

// Brief scopes — Week is the default; Month covers the calendar month.
const (
	ReportWeek  ReportRange = 0
	ReportMonth ReportRange = 1
)

// Aggregate computes Stats over the given DayRecords. Order doesn't matter —
// records are sorted internally before walking. "Streak" counts back from
// the newest workday with a session.
//
// listDayOffs returns the day-offs in the given inclusive date range; it
// is invoked once per Aggregate call to populate Stats.DaysOff. Pass a
// closure that wraps a DayOffStore at the use-case boundary.
func Aggregate(
	records []DayRecord,
	isWorkday func(time.Time) bool,
	listDayOffs func(from, to time.Time) []DayOff,
) Stats {
	if len(records) == 0 {
		return Stats{ByTag: map[string]time.Duration{}, CountByTag: map[string]int{}}
	}
	sorted := make([]DayRecord, len(records))
	copy(sorted, records)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })

	st := Stats{Days: len(sorted), ByTag: map[string]time.Duration{}, CountByTag: map[string]int{}}
	// Min initialises lazily so empty (Total == 0) placeholder days
	// don't dominate the result. Without this, a range starting with an
	// untracked weekday placeholder would pin Min = 0 forever even
	// though the user worked 4–8h on every other day.
	minSeen := false

	for _, r := range sorted {
		st.Total += r.Total
		if r.Total > st.Max {
			st.Max = r.Total
			st.MaxDate = r.Date
		}
		if r.Total > 0 {
			st.DaysWithSessions++
			if !minSeen || r.Total < st.Min {
				st.Min = r.Total
				st.MinDate = r.Date
				minSeen = true
			}
		}
		if isWorkday(r.Date) {
			st.Workdays++
			if r.Total >= r.Target {
				st.Hits++
			}
			st.Overtime += r.Total - r.Target
		}
		for _, s := range r.Sessions {
			st.ByTag[s.Tag] += s.Elapsed
			st.CountByTag[s.Tag]++
		}
	}
	if st.DaysWithSessions > 0 {
		st.Avg = st.Total / time.Duration(st.DaysWithSessions)
	}
	st.Untagged = st.ByTag[""]

	st.BestStreak = bestStreak(sorted, isWorkday)
	st.Streak = currentStreak(sorted, isWorkday)

	if listDayOffs != nil {
		st.DaysOff = listDayOffs(sorted[0].Date, sorted[len(sorted)-1].Date)
	}
	return st
}

// bestStreak walks forward over workdays-with-target-met and returns the
// longest run. Non-workdays are transparent — they neither extend nor
// break the streak.
func bestStreak(sorted []DayRecord, isWorkday func(time.Time) bool) int {
	cur, best := 0, 0
	for _, r := range sorted {
		if !isWorkday(r.Date) {
			continue
		}
		if r.Total >= r.Target {
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 0
		}
	}
	return best
}

// currentStreak walks backward from the newest workday and counts hits
// until the first miss. Non-workdays are transparent.
func currentStreak(sorted []DayRecord, isWorkday func(time.Time) bool) int {
	streak := 0
	for i := len(sorted) - 1; i >= 0; i-- {
		r := sorted[i]
		if !isWorkday(r.Date) {
			continue
		}
		if r.Total >= r.Target {
			streak++
		} else {
			break
		}
	}
	return streak
}

// FilterRecords keeps records whose date is in [from, to). Helper for
// week/month aggregations. Both bounds are required.
func FilterRecords(records []DayRecord, from, to time.Time) []DayRecord {
	out := make([]DayRecord, 0, len(records))
	for _, r := range records {
		if !r.Date.Before(from) && r.Date.Before(to) {
			out = append(out, r)
		}
	}
	return out
}

// BriefBounds resolves the [from, to) span and the title for a brief.
// from is inclusive, to is exclusive (one day past the brief's last day).
// Keep the half-open contract stable — callers in usecase/reporter.go
// compensate with `to.AddDate(0, 0, -1)` when the downstream API wants
// inclusive bounds (DayOffStore.List), so changing this signature is a
// silent off-by-one bug if every call site isn't updated together.
func BriefBounds(ref time.Time, scope ReportRange) (from, to time.Time, title string) {
	switch scope {
	case ReportMonth:
		from = time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, ref.Location())
		to = from.AddDate(0, 1, 0)
		title = fmt.Sprintf("Worktime · %s %d", MonthShortDe(ref.Month()), ref.Year())
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

// PlannedTarget sums targetFor over all workdays in [from, to). Day-offs
// reduce the planned target — that's how the saldo line stays meaningful
// in vacation weeks.
func PlannedTarget(
	from, to time.Time,
	isWorkday func(time.Time) bool,
	targetFor func(time.Time) time.Duration,
) time.Duration {
	var sum time.Duration
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		if !isWorkday(d) {
			continue
		}
		sum += targetFor(d)
	}
	return sum
}

// MonthBurndownCompute summarises progress against the implied monthly
// target (sum of targetFor(d) over all workdays in the month, day-offs
// subtracted). "Expected" sums targetFor over workdays whose date is
// strictly before today — today itself doesn't count toward expected,
// giving the user the full day to hit it before the gauge turns yellow.
//
// The active session, if it falls in the month, contributes its live tail
// to Total — clamped to today's midnight when it crossed midnight.
func MonthBurndownCompute(
	now time.Time,
	records []DayRecord,
	active *time.Time,
	isWorkday func(time.Time) bool,
	targetFor func(time.Time) time.Duration,
) MonthBurndownReport {
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	to := from.AddDate(0, 1, 0)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	rep := MonthBurndownReport{}
	var expected time.Duration
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		if !isWorkday(d) {
			continue
		}
		t := targetFor(d)
		rep.WorkdaysAll++
		rep.Target += t
		if d.Before(today) {
			expected += t
			rep.WorkdaysDue++
		}
	}

	for _, r := range records {
		if !r.Date.Before(from) && r.Date.Before(to) {
			rep.Total += r.Total
		}
	}

	if active != nil && !active.Before(from) && active.Before(to) {
		start := *active
		if start.Before(today) {
			start = today
		}
		rep.Total += now.Sub(start)
	}

	rep.Saldo = rep.Total - expected
	rep.OnTrack = rep.Saldo >= 0
	return rep
}
