package browse

// Browse filter + search predicates — applyFilters recomputes m.visible
// aus m.all unter Type-Filter und Suchquery; matchesFilter/matchesQuery
// sind die kleinen Prädikate dahinter. Split aus model.go (Skill
// §No-Monoliths): Filter-Logik bleibt eigenständig, weil sie sowohl
// vom Update-Reducer (nach entries/bodiesLoaded und in Suche/Filter-
// Toggles) als auch von Tests verwendet wird.

import (
	"strings"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// applyFilters recomputes the visible slice from m.all under the current
// filter + search query, clamping the cursor into the new range.
func (m *Model) applyFilters() {
	q := strings.ToLower(m.search.Value())
	m.visible = m.visible[:0]
	for _, e := range m.all {
		if !matchesFilter(e, m.filter) {
			continue
		}
		if !m.matchesQuery(e, q) {
			continue
		}
		m.visible = append(m.visible, e)
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
}

func matchesFilter(e ports.NoteEntry, f Filter) bool {
	switch f {
	case FilterDaily:
		return e.Meta.Type == domain.TypeDaily
	case FilterProject:
		return e.Meta.Type == domain.TypeProject
	case FilterFree:
		return e.Meta.Type == domain.TypeFree
	}
	return true
}

// matchesQuery searches the note body first (the user's intent) and then
// the title + project. The auto-generated ID/path is intentionally skipped
// — those are scheme-derived noise that drowns real matches.
func (m *Model) matchesQuery(e ports.NoteEntry, q string) bool {
	if q == "" {
		return true
	}
	if body, ok := m.bodies[e.ID]; ok {
		if strings.Contains(strings.ToLower(string(body)), q) {
			return true
		}
	}
	for _, h := range []string{
		strings.ToLower(e.Meta.Title),
		strings.ToLower(e.Meta.Project),
	} {
		if h != "" && strings.Contains(h, q) {
			return true
		}
	}
	return false
}
