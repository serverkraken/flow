package usecase

import (
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// LinkReader returns the Kompendium-note IDs attached to a date. Thin
// wrapper kept for the use-case-as-API discipline.
type LinkReader struct {
	Store ports.LinkStore
}

// ListByDate returns the note IDs attached to date, in insertion order.
func (r *LinkReader) ListByDate(date time.Time) ([]string, error) {
	return r.Store.ListByDate(date)
}

// CountsByDate returns the per-date attachment counts (key YYYY-MM-DD).
// Surfaces (history list/heatmap/month) call this once per reload to
// flag „dieser Tag hat Notes" without N file-reads.
func (r *LinkReader) CountsByDate() (map[string]int, error) {
	return r.Store.CountsByDate()
}
