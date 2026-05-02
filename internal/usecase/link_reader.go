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
