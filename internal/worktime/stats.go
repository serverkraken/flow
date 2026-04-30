package worktime

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Stats summarizes a slice of DayRecords (any time range).
//
// Workday-counting rules (consistent across all fields):
//
//   - "Workday" = neither weekend nor a configured day-off (Feiertag/Urlaub/Krank).
//   - Hits, Streak, BestStreak, Overtime are computed over Workdays only.
//   - Days/Total/Avg/Max/Min still cover *all* records in the input — that
//     stays useful for "total tracked time" reporting.
//   - DaysOff lists the configured day-offs that fell into the range.
type Stats struct {
	Days       int
	Workdays   int
	Total      time.Duration
	Avg        time.Duration // Total / Days (only days with sessions)
	Max        time.Duration
	MaxDate    time.Time
	Min        time.Duration
	MinDate    time.Time
	Hits       int           // workdays that met or exceeded their target
	Streak     int           // current consecutive-from-newest hit streak (workdays only)
	BestStreak int           // longest run of consecutive hits ever (workdays only)
	Overtime   time.Duration // sum of (Total - Target) over workdays only

	// ByTag maps tag → total duration over the input. Sessions with empty
	// Tag are aggregated under the empty key "" — callers that want a strip
	// should skip that bucket.
	ByTag map[string]time.Duration

	// Untagged is the total of sessions without a Tag. Equivalent to
	// ByTag[""], hoisted to a top-level field because UI surfaces it
	// explicitly ("untagged 2h 15m" line in History header).
	Untagged time.Duration

	// DaysOff is the configured day-offs (Feiertag/Urlaub/Krank) that fall
	// within [first record date, last record date]. Empty when the input
	// is empty.
	DaysOff []DayOff
}

// Aggregate computes stats over the given DayRecords. Order doesn't matter.
// "Streak" counts back from the newest workday with a session.
func Aggregate(records []DayRecord) Stats {
	if len(records) == 0 {
		return Stats{ByTag: map[string]time.Duration{}}
	}
	sorted := make([]DayRecord, len(records))
	copy(sorted, records)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })

	st := Stats{Days: len(sorted), ByTag: map[string]time.Duration{}}
	st.Min = sorted[0].Total
	st.MinDate = sorted[0].Date

	for _, r := range sorted {
		st.Total += r.Total
		if r.Total > st.Max {
			st.Max = r.Total
			st.MaxDate = r.Date
		}
		if r.Total < st.Min {
			st.Min = r.Total
			st.MinDate = r.Date
		}
		isWork := IsWorkday(r.Date)
		if isWork {
			st.Workdays++
			if r.Total >= r.Target {
				st.Hits++
			}
			st.Overtime += r.Total - r.Target
		}
		for _, s := range r.Sessions {
			st.ByTag[s.Tag] += s.Elapsed
		}
	}
	if st.Days > 0 {
		st.Avg = st.Total / time.Duration(st.Days)
	}
	st.Untagged = st.ByTag[""]

	// Best streak: walk forward over workdays only.
	cur := 0
	for _, r := range sorted {
		if !IsWorkday(r.Date) {
			continue
		}
		if r.Total >= r.Target {
			cur++
			if cur > st.BestStreak {
				st.BestStreak = cur
			}
		} else {
			cur = 0
		}
	}

	// Current streak: walk backward over workdays only from newest.
	for i := len(sorted) - 1; i >= 0; i-- {
		r := sorted[i]
		if !IsWorkday(r.Date) {
			continue
		}
		if r.Total >= r.Target {
			st.Streak++
		} else {
			break
		}
	}

	st.DaysOff = ListDayOffs(sorted[0].Date, sorted[len(sorted)-1].Date)
	return st
}

// CurrentStreak walks the on-disk history and reports the current workday
// streak ending today (or the most recent workday with sessions).
// Returns 0 on read failure rather than an error — callers (status segment,
// header) treat this as best-effort eye candy.
func CurrentStreak() int {
	hist, err := LoadHistory()
	if err != nil || len(hist) == 0 {
		return 0
	}
	st := Aggregate(hist)
	return st.Streak
}

// IsWorkday reports whether t is neither a weekend nor a configured day-off.
func IsWorkday(t time.Time) bool {
	if isWeekend(t) {
		return false
	}
	if IsDayOff(t) {
		return false
	}
	return true
}

func isWeekend(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// TopTags returns the top n tags by duration, descending. Empty-tag bucket
// is excluded. Used by the history-header tag strip.
func (s Stats) TopTags(n int) []TagDur {
	out := make([]TagDur, 0, len(s.ByTag))
	for k, v := range s.ByTag {
		if k == "" {
			continue
		}
		out = append(out, TagDur{Tag: k, Total: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Tag < out[j].Tag
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// TagDur is a single (tag, duration) pair.
type TagDur struct {
	Tag   string
	Total time.Duration
}

// WeekStats aggregates the ISO-week containing ref. Returns zero-value Stats
// on read error — callers display fallback labels rather than propagate.
func WeekStats(ref time.Time) Stats {
	hist, err := LoadHistory()
	if err != nil {
		return Stats{ByTag: map[string]time.Duration{}}
	}
	mon := isoMonday(ref)
	sun := mon.AddDate(0, 0, 7)
	return Aggregate(filterRecords(hist, mon, sun))
}

// MonthStats aggregates the calendar month containing ref.
func MonthStats(ref time.Time) Stats {
	hist, err := LoadHistory()
	if err != nil {
		return Stats{ByTag: map[string]time.Duration{}}
	}
	from := time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, ref.Location())
	to := from.AddDate(0, 1, 0)
	return Aggregate(filterRecords(hist, from, to))
}

// MonthBurndown summarizes progress against the implied monthly target
// (sum of TargetFor(d) over all workdays in the month, day-offs subtracted).
//
// Used by the Today header to show "Monat 78h / 160h · vorne 2h" — a glance
// at whether the running balance is healthy.
type MonthBurndownReport struct {
	Total       time.Duration // logged so far this month (incl. today logged + active)
	Target      time.Duration // sum of targets over all workdays of the month
	Saldo       time.Duration // Total - expected-by-now
	OnTrack     bool          // Saldo >= 0
	WorkdaysAll int
	WorkdaysDue int // workdays whose date <= today
}

// MonthBurndown computes the report for the month containing now.
// "expected" = sum of TargetFor over workdays whose date is strictly before
// today (today itself doesn't count toward expected — gives the user the full
// day to hit it before the gauge turns yellow).
func MonthBurndown(now time.Time) MonthBurndownReport {
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	to := from.AddDate(0, 1, 0)

	rep := MonthBurndownReport{}
	var expected time.Duration
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		if !IsWorkday(d) {
			continue
		}
		t := TargetFor(d)
		rep.WorkdaysAll++
		rep.Target += t
		if d.Before(today) {
			expected += t
			rep.WorkdaysDue++
		}
	}

	hist, _ := LoadHistory()
	for _, r := range hist {
		if !r.Date.Before(from) && r.Date.Before(to) {
			rep.Total += r.Total
		}
	}

	// Active session contributes too — the user wants to see overtime accumulate
	// in real-time rather than only after Stop is pressed.
	if active, _ := readActiveStateForBurndown(); active != nil && !active.Before(from) && active.Before(to) {
		// Today's logged is already in rep.Total via LoadHistory (which stops
		// at logged sessions); add the live tail.
		mid := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		start := *active
		if start.Before(mid) {
			start = mid
		}
		rep.Total += now.Sub(start)
	}

	rep.Saldo = rep.Total - expected
	rep.OnTrack = rep.Saldo >= 0
	return rep
}

// readActiveStateForBurndown is a thin wrapper that reads the worktime.state
// file. Returns nil on any error (idle case).
func readActiveStateForBurndown() (*time.Time, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return readActiveState(filepath.Join(home, ".tmux", "worktime.state"))
}

// isoMonday returns the Monday 00:00 of t's ISO week, in t's location.
func isoMonday(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	d := t.AddDate(0, 0, -(wd - 1))
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, t.Location())
}

// filterRecords keeps records whose date is in [from, to). Helper for the
// WeekStats / MonthStats top-level entry points.
func filterRecords(records []DayRecord, from, to time.Time) []DayRecord {
	out := make([]DayRecord, 0, len(records))
	for _, r := range records {
		if !r.Date.Before(from) && r.Date.Before(to) {
			out = append(out, r)
		}
	}
	return out
}
