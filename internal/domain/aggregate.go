package domain

import (
	"sort"
	"time"
)

// isHit reports whether a day-record counts as a target-hit for Stats.
// Single owner of the rule so Aggregate and AggregateRange can't drift
// apart on edge cases (e.g. Target == 0 days). Both callers required
// Target > 0 historically — Aggregate's branch dropped the guard and a
// workday with explicit Target = 0 counted as a hit there but not in
// AggregateRange. Documenting the rule here: a hit requires a positive
// Target AND Total ≥ Target.
func isHit(total, target time.Duration) bool {
	return target > 0 && total >= target
}

// Aggregate computes Stats over the given DayRecords. Order doesn't matter —
// records are sorted internally before walking. "Streak" counts back from
// the newest workday with a session.
//
// listDayOffs returns the day-offs in the given inclusive date range; it
// is invoked once per Aggregate call to populate Stats.DaysOff. Pass a
// closure that wraps a DayOffStore at the use-case boundary.
//
// IMPORTANT: Aggregate's saldo (Stats.Overtime) and Workdays count only
// the *days that produced records*. A workday with zero sessions has no
// record and therefore contributes nothing to either. Use AggregateRange
// when the caller knows a fixed [from, to) span and wants missed
// workdays to count toward the saldo (status segment, week/month brief,
// `flow worktime stats <range>`); use Aggregate for filtered subsets
// where the span is implicit (the history.go tag/note filters).
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
		st.TargetTotal += r.TargetTotal
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
			if isHit(r.TargetTotal, r.Target) {
				st.Hits++
			}
			st.Overtime += r.TargetTotal - r.Target
		}
		for _, s := range r.Sessions {
			tallySessionTags(&st, s)
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
		if r.TargetTotal >= r.Target {
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
// until the first miss. Non-workdays are transparent. NOTE: this
// only walks the records — a workday between records that produced
// no session is invisible and does NOT break the streak. The
// AggregateRange caller can compensate by filling in zero-Total
// placeholders for unworked workdays before invoking this helper.
func currentStreak(sorted []DayRecord, isWorkday func(time.Time) bool) int {
	streak := 0
	for i := len(sorted) - 1; i >= 0; i-- {
		r := sorted[i]
		if !isWorkday(r.Date) {
			continue
		}
		if r.TargetTotal >= r.Target {
			streak++
		} else {
			break
		}
	}
	return streak
}

// AggregateRange computes Stats over [from, to) (half-open). Differs
// from Aggregate in that it walks every workday in the range — a
// workday without a record contributes 0h Total and the configured
// targetFor(d) toward the saldo, so a partial week with two unworked
// days correctly shows e.g. -16h Overtime. listDayOffs is called over
// the inclusive [from, to-1d] range to populate Stats.DaysOff.
//
// records may include rows outside [from, to); they are filtered. Same
// invariants as Aggregate: order doesn't matter, sessions are walked
// for the by-tag tally, the active session (if any) belongs in records
// already if the caller wants it counted.
func AggregateRange(
	records []DayRecord,
	from, to time.Time,
	isWorkday func(time.Time) bool,
	targetFor func(time.Time) time.Duration,
	listDayOffs func(from, to time.Time) []DayOff,
) Stats {
	st := Stats{ByTag: map[string]time.Duration{}, CountByTag: map[string]int{}}
	if !from.Before(to) {
		return st
	}
	inRange, byDate := filterAndIndexRange(records, from, to)
	st.Days = len(inRange)
	tallyRecordsInto(&st, inRange)
	walkWorkdaysForSaldo(&st, from, to, byDate, isWorkday, targetFor)

	if st.DaysWithSessions > 0 {
		st.Avg = st.Total / time.Duration(st.DaysWithSessions)
	}
	st.Untagged = st.ByTag[""]
	st.BestStreak = bestStreak(inRange, isWorkday)
	st.Streak = currentStreak(inRange, isWorkday)

	if listDayOffs != nil {
		// to is exclusive; ListDayOffs takes inclusive bounds.
		st.DaysOff = listDayOffs(from, to.AddDate(0, 0, -1))
	}
	return st
}

// filterAndIndexRange returns records within [from, to) sorted
// chronologically plus a date-keyed index for the workday walk.
func filterAndIndexRange(records []DayRecord, from, to time.Time) ([]DayRecord, map[time.Time]DayRecord) {
	inRange := make([]DayRecord, 0, len(records))
	for _, r := range records {
		if !r.Date.Before(from) && r.Date.Before(to) {
			inRange = append(inRange, r)
		}
	}
	sort.Slice(inRange, func(i, j int) bool { return inRange[i].Date.Before(inRange[j].Date) })
	byDate := make(map[time.Time]DayRecord, len(inRange))
	for _, r := range inRange {
		byDate[truncDay(r.Date)] = r
	}
	return inRange, byDate
}

// tallyRecordsInto walks each record's sessions and accumulates totals,
// max/min, by-tag duration and CountByTag counts onto st.
func tallyRecordsInto(st *Stats, inRange []DayRecord) {
	minSeen := false
	for _, r := range inRange {
		st.Total += r.Total
		st.TargetTotal += r.TargetTotal
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
		for _, s := range r.Sessions {
			tallySessionTags(st, s)
		}
	}
}

// tallySessionTags adds a session's elapsed time to each of its tags' buckets.
// A multi-tag session adds its full elapsed to EACH tag (per-tag totals can
// overlap — that's the correct "time touching tag X" semantics). A session with
// no tags contributes to the "" (untagged) bucket so Stats.Untagged stays
// meaningful (mirrors the pre-cutover `ByTag[s.Tag]` where s.Tag was "").
func tallySessionTags(st *Stats, s RecordSession) {
	if len(s.Tags) == 0 {
		st.ByTag[""] += s.Elapsed
		st.CountByTag[""]++
		return
	}
	for _, t := range s.Tags {
		st.ByTag[t] += s.Elapsed
		st.CountByTag[t]++
	}
}

// walkWorkdaysForSaldo iterates every workday in [from, to) — recorded
// or not — and feeds Workdays/Hits/Overtime so the saldo accounts for
// unworked workdays. Streak semantics deliberately stay record-driven
// (consistent with Aggregate) — a workday-aware streak that breaks on
// missed workdays needs `now` to distinguish past misses from future
// days, which AggregateRange does not take.
func walkWorkdaysForSaldo(
	st *Stats,
	from, to time.Time,
	byDate map[time.Time]DayRecord,
	isWorkday func(time.Time) bool,
	targetFor func(time.Time) time.Duration,
) {
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		if !isWorkday(d) {
			continue
		}
		st.Workdays++
		if rec, ok := byDate[truncDay(d)]; ok {
			if isHit(rec.TargetTotal, rec.Target) {
				st.Hits++
			}
			st.Overtime += rec.TargetTotal - rec.Target
			continue
		}
		st.Overtime -= targetFor(d)
	}
}

// truncDay strips the time-of-day component so two timestamps on the
// same calendar day compare equal as map keys.
func truncDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
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
			rep.TargetTotal += r.TargetTotal
		}
	}

	if active != nil && !active.Before(from) && active.Before(to) {
		start := *active
		if start.Before(today) {
			start = today
		}
		rep.Total += now.Sub(start)
	}

	rep.Saldo = rep.TargetTotal - expected
	rep.OnTrack = rep.Saldo >= 0
	return rep
}
