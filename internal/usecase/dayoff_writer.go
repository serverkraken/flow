package usecase

import (
	"errors"
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DayOffWriter is the action surface for day-off (Feiertag/Urlaub/Krank)
// management. The store handles cache invalidation; the use case validates
// the kind, sanitises the label, and orchestrates batch operations.
type DayOffWriter struct {
	Store ports.DayOffStore
}

// Add inserts or replaces an entry for date. An empty label defaults to
// the kind's German label ("Feiertag" / "Urlaub" / "Krank") so the row
// always renders meaningfully.
func (w *DayOffWriter) Add(date time.Time, kind domain.Kind, label string) error {
	if _, ok := domain.ParseKind(string(kind)); !ok {
		return fmt.Errorf("ungültige kategorie: %q", kind)
	}
	label = sanitizeField(label)
	if label == "" {
		label = kind.LabelDe()
	}
	return w.Store.Add(domain.DayOff{
		Date:  startOfDay(date),
		Kind:  kind,
		Label: label,
	})
}

// AddRange adds an entry for every calendar day in [from, to] (inclusive).
// Returns the number of days actually written; on partial failure, the
// count reflects how many succeeded before the error.
func (w *DayOffWriter) AddRange(from, to time.Time, kind domain.Kind, label string) (int, error) {
	if to.Before(from) {
		return 0, errors.New("to liegt vor from")
	}
	cur := startOfDay(from)
	end := startOfDay(to)
	count := 0
	for !cur.After(end) {
		if err := w.Add(cur, kind, label); err != nil {
			return count, err
		}
		count++
		cur = cur.AddDate(0, 0, 1)
	}
	return count, nil
}

// Remove deletes the entry for date. Removing a non-existent entry is a
// no-op (the store handles that contract).
func (w *DayOffWriter) Remove(date time.Time) error {
	return w.Store.Remove(date)
}

// SyncGermanHolidays adds (or skips) entries for every gesetzliche
// Feiertag of year + Bundesland. Idempotent: re-running over an already-
// synced year is a no-op. User-managed kinds (vacation / sick) are
// preserved — those get counted as "skipped" so the caller can report
// "1 added, 5 already had user entries".
func (w *DayOffWriter) SyncGermanHolidays(year int, land string) (added, skipped int, err error) {
	hs := domain.GermanHolidays(year, land, time.Local)
	for _, h := range hs {
		existing, ok := w.Store.Lookup(h.Date)
		if ok && existing.Kind == domain.KindHoliday && existing.Label == h.Label {
			skipped++
			continue
		}
		if ok && existing.Kind != domain.KindHoliday {
			// Don't overwrite vacation/sick — user intent wins.
			skipped++
			continue
		}
		if err := w.Store.Add(domain.DayOff{Date: h.Date, Kind: h.Kind, Label: h.Label}); err != nil {
			return added, skipped, err
		}
		added++
	}
	return added, skipped, nil
}
