// Package fuzzymatch is a tiny case-insensitive subsequence matcher with match
// indices and a quality score, for filterable lists. Domain-free, no deps.
package fuzzymatch

import "strings"

// Match reports whether query is a case-insensitive subsequence of target.
// idx are the rune indices in target that matched (for highlight); a higher
// score means a tighter match (contiguous and early runes score more). An empty
// query matches everything (ok=true, idx=nil, score=0). When not all query runes
// are found, ok=false and idx=nil.
func Match(query, target string) (idx []int, score int, ok bool) {
	if query == "" {
		return nil, 0, true
	}
	q := []rune(strings.ToLower(query))
	tl := []rune(strings.ToLower(target))
	qi := 0
	prev := -2
	for ti := 0; ti < len(tl) && qi < len(q); ti++ {
		if tl[ti] != q[qi] {
			continue
		}
		idx = append(idx, ti)
		score += 10
		if ti == prev+1 {
			score += 5 // contiguous with previous match
		}
		if ti == 0 {
			score += 5 // matches at the very start
		}
		prev = ti
		qi++
	}
	if qi < len(q) {
		return nil, 0, false
	}
	// Prefer tighter targets: subtract the unmatched-length slack.
	score -= len(tl) - len(idx)
	return idx, score, true
}
