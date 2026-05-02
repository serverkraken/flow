package domain

import (
	"sort"
	"strings"
	"time"
)

// RecentTags returns up to n distinct tags most recently used, newest
// first. "Recency" is defined by Session.Start. Empty tags are skipped.
// Used by the TUI tag-form for autocomplete.
func RecentTags(sessions []Session, n int) []string {
	if n <= 0 {
		return nil
	}
	sorted := make([]Session, len(sessions))
	copy(sorted, sessions)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start.After(sorted[j].Start) })

	seen := make(map[string]bool, n)
	out := make([]string, 0, n)
	for _, s := range sorted {
		if s.Tag == "" || seen[s.Tag] {
			continue
		}
		seen[s.Tag] = true
		out = append(out, s.Tag)
		if len(out) >= n {
			break
		}
	}
	return out
}

// TopUsageTags returns up to n distinct tags ordered by total session
// count (descending). Ties are broken by most recent use. Used by the
// TUI tag-form to show a "top by usage" suggestion strip alongside the
// recency strip.
func TopUsageTags(sessions []Session, n int) []string {
	if n <= 0 {
		return nil
	}
	type entry struct {
		tag    string
		count  int
		latest time.Time
	}
	bucket := make(map[string]*entry, len(sessions))
	for _, s := range sessions {
		if s.Tag == "" {
			continue
		}
		e, ok := bucket[s.Tag]
		if !ok {
			e = &entry{tag: s.Tag}
			bucket[s.Tag] = e
		}
		e.count++
		if s.Start.After(e.latest) {
			e.latest = s.Start
		}
	}
	all := make([]*entry, 0, len(bucket))
	for _, e := range bucket {
		all = append(all, e)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].count != all[j].count {
			return all[i].count > all[j].count
		}
		return all[i].latest.After(all[j].latest)
	})
	out := make([]string, 0, n)
	for _, e := range all {
		out = append(out, e.tag)
		if len(out) >= n {
			break
		}
	}
	return out
}

// SessionTemplatesOf buckets the input sessions by (start-of-day rounded
// to 15 min, duration rounded to 15 min, tag) and returns the top-n shapes
// with count >= 2, ordered by count desc then most-recent.
//
// The 15-minute grid is wide enough to absorb typical drift ("standup
// ~9:30 give or take a few minutes") yet narrow enough that patterns of
// different kinds don't merge. Sessions crossing midnight and sessions
// shorter than the grid are excluded.
func SessionTemplatesOf(sessions []Session, n int) []SessionTemplate {
	if n <= 0 {
		return nil
	}
	const grid = 15 * time.Minute
	roundDown := func(d time.Duration) time.Duration { return (d / grid) * grid }
	type key struct {
		start time.Duration
		dur   time.Duration
		tag   string
	}
	bucket := make(map[key]*SessionTemplate, len(sessions))
	for _, s := range sessions {
		if s.Start.Day() != s.Stop.Day() {
			continue
		}
		startOff := time.Duration(s.Start.Hour())*time.Hour +
			time.Duration(s.Start.Minute())*time.Minute
		startB := roundDown(startOff)
		durB := roundDown(s.Elapsed)
		if durB < grid {
			continue
		}
		k := key{start: startB, dur: durB, tag: strings.ToLower(s.Tag)}
		t, ok := bucket[k]
		if !ok {
			t = &SessionTemplate{Start: startB, Duration: durB, Tag: s.Tag}
			bucket[k] = t
		}
		t.Count++
		if s.Start.After(t.Latest) {
			t.Latest = s.Start
			// Preserve the original tag casing of the most recent occurrence.
			t.Tag = s.Tag
		}
	}
	out := make([]SessionTemplate, 0, len(bucket))
	for _, t := range bucket {
		if t.Count < 2 {
			continue
		}
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Latest.After(out[j].Latest)
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
