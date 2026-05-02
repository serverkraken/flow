package usecase

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DayOffReader is the read-only entry point for day-off lookups. Thin
// wrapper around the store, but kept as a separate type so dependency
// composition stays uniform across use cases.
type DayOffReader struct {
	Store ports.DayOffStore
}

// List returns all entries in [from, to] (inclusive on both ends),
// sorted ascending by date. Zero from/to means "no bound on that side".
func (r *DayOffReader) List(from, to time.Time) []domain.DayOff {
	return r.Store.List(from, to)
}

// Lookup returns the entry for date and a found flag.
func (r *DayOffReader) Lookup(date time.Time) (domain.DayOff, bool) {
	return r.Store.Lookup(date)
}
