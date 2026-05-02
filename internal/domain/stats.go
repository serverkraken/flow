package domain

import (
	"sort"
	"time"
)

// Stats summarises a slice of DayRecords (any time range).
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

	// CountByTag mirrors ByTag with the per-tag session count. Lets the UI
	// show "deep: 4×, ⌀ 1h45m" without re-walking the records.
	CountByTag map[string]int

	// Untagged is the total of sessions without a Tag. Equivalent to
	// ByTag[""], hoisted to a top-level field because the UI surfaces it
	// explicitly ("untagged 2h 15m" line in the History header).
	Untagged time.Duration

	// DaysOff is the configured day-offs (Feiertag/Urlaub/Krank) that fall
	// within [first record date, last record date]. Empty when the input
	// is empty.
	DaysOff []DayOff
}

// TagDur is a (tag, total duration, session count) triple. Count lets the UI
// derive an average per-session duration without re-aggregating.
type TagDur struct {
	Tag   string
	Total time.Duration
	Count int
}

// TopTags returns the top n tags by duration, descending. Empty-tag bucket
// is excluded. n <= 0 means "no limit".
//
// Ties are broken by tag name ascending so the order is stable across runs.
func (s Stats) TopTags(n int) []TagDur {
	out := make([]TagDur, 0, len(s.ByTag))
	for k, v := range s.ByTag {
		if k == "" {
			continue
		}
		out = append(out, TagDur{Tag: k, Total: v, Count: s.CountByTag[k]})
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
