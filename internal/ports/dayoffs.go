package ports

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// DayOffStore persists Feiertag/Urlaub/Krank entries. Reads are cached by
// the adapter; writes invalidate the cache automatically.
type DayOffStore interface {
	// List returns all entries in [from, to] (inclusive on both ends),
	// sorted ascending by date. Zero from/to means "no bound on that side".
	List(from, to time.Time) []domain.DayOff
	// Lookup returns the entry for date and a found flag.
	Lookup(date time.Time) (domain.DayOff, bool)
	// Add inserts or replaces the entry for the given date.
	Add(off domain.DayOff) error
	// Remove deletes the entry for date. Removing a non-existent entry
	// is a no-op.
	Remove(date time.Time) error
}
