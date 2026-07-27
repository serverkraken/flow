package worktime

import (
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// mruEngagements filters nodes to engagements only, then orders them by
// most-recently-used. A node's recency is the latest session referencing it
// (Stop if set, else Start). Unused engagements keep their original relative
// order and trail the used ones. Pure.
func mruEngagements(nodes []domain.Node, sessions []domain.WorkSession) []domain.Node {
	// Filter to engagements only.
	var engs []domain.Node
	for _, n := range nodes {
		if n.Kind == domain.KindEngagement {
			engs = append(engs, n)
		}
	}
	last := make(map[string]time.Time, len(engs))
	for _, s := range sessions {
		if s.NodeID == nil {
			continue
		}
		t := s.Start
		if s.Stop != nil {
			t = *s.Stop
		}
		if cur, ok := last[*s.NodeID]; !ok || t.After(cur) {
			last[*s.NodeID] = t
		}
	}
	idxOf := make(map[string]int, len(engs))
	for i, n := range engs {
		idxOf[n.ID] = i
	}
	out := append([]domain.Node(nil), engs...)
	sort.SliceStable(out, func(a, b int) bool {
		ta, oka := last[out[a].ID]
		tb, okb := last[out[b].ID]
		if oka != okb {
			return oka // used engagements come first
		}
		if oka && okb && !ta.Equal(tb) {
			return ta.After(tb) // more recent first
		}
		return idxOf[out[a].ID] < idxOf[out[b].ID] // stable original order
	})
	return out
}
