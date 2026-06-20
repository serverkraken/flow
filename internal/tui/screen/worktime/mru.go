package worktime

import (
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// mruProjects orders projects by most-recently-used. A project's recency is the
// latest session referencing it (Stop if set, else Start). Projects with no
// session keep their original relative order and trail the used ones. Pure.
func mruProjects(projects []domain.Project, sessions []domain.WorkSession) []domain.Project {
	last := make(map[string]time.Time, len(projects))
	for _, s := range sessions {
		if s.ProjectID == nil {
			continue
		}
		t := s.Start
		if s.Stop != nil {
			t = *s.Stop
		}
		if cur, ok := last[*s.ProjectID]; !ok || t.After(cur) {
			last[*s.ProjectID] = t
		}
	}
	idxOf := make(map[string]int, len(projects))
	for i, p := range projects {
		idxOf[p.ID] = i
	}
	out := append([]domain.Project(nil), projects...)
	sort.SliceStable(out, func(a, b int) bool {
		ta, oka := last[out[a].ID]
		tb, okb := last[out[b].ID]
		if oka != okb {
			return oka // used projects come first
		}
		if oka && okb && !ta.Equal(tb) {
			return ta.After(tb) // more recent first
		}
		return idxOf[out[a].ID] < idxOf[out[b].ID] // stable original order
	})
	return out
}
