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
//
// A label containing tab/CR/LF is rejected with an explicit error so
// the user notices their shell escaping went wrong — silently turning
// `"first\nsecond"` into `"first second"` was lossy without feedback.
// Surrounding whitespace is still trimmed silently (intentional UX).
func (w *DayOffWriter) Add(date time.Time, kind domain.Kind, label string) error {
	if _, ok := domain.ParseKind(string(kind)); !ok {
		return fmt.Errorf("ungültige kategorie: %q", kind)
	}
	if containsTSVMeta(label) {
		return fmt.Errorf("label enthält Tab oder Zeilenumbruch — bitte ohne diese Zeichen eingeben")
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

// containsTSVMeta reports whether s contains any of the characters that
// would otherwise be replaced silently by sanitizeField. Whitespace at
// the edges is fine — sanitizeField TrimSpaces it without complaint.
func containsTSVMeta(s string) bool {
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' {
			return true
		}
	}
	return false
}

// AddRange adds an entry for every calendar day in [from, to] (inclusive).
// All or nothing: the underlying store's AddBatch is atomic, so a
// disk-full or permission error on day 7 of 10 leaves zero rows written
// — the previous per-day-Add loop left partial state with no signal
// which days survived. On success returns the number of days written.
//
// Validation (kind, label) runs once for the whole range — the
// previous per-day path duplicated this work and could fail in the
// middle. Failure paths now return (0, err).
func (w *DayOffWriter) AddRange(from, to time.Time, kind domain.Kind, label string) (int, error) {
	if to.Before(from) {
		return 0, errors.New("to liegt vor from")
	}
	if _, ok := domain.ParseKind(string(kind)); !ok {
		return 0, fmt.Errorf("ungültige kategorie: %q", kind)
	}
	if containsTSVMeta(label) {
		return 0, fmt.Errorf("label enthält Tab oder Zeilenumbruch — bitte ohne diese Zeichen eingeben")
	}
	label = sanitizeField(label)
	if label == "" {
		label = kind.LabelDe()
	}
	var offs []domain.DayOff
	for cur, end := startOfDay(from), startOfDay(to); !cur.After(end); cur = cur.AddDate(0, 0, 1) {
		offs = append(offs, domain.DayOff{Date: cur, Kind: kind, Label: label})
	}
	if err := w.Store.AddBatch(offs); err != nil {
		return 0, err
	}
	return len(offs), nil
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
//
// loc anchors holiday dates at midnight of that location. Production
// callers (CLI / TUI) pass time.Local because that's what the user's
// calendar sees; tests pass an explicit location so the assertions
// don't drift under CI environments with a different $TZ. The
// previous hardcoded time.Local broke that injection contract.
func (w *DayOffWriter) SyncGermanHolidays(year int, land string, loc *time.Location) (added, skipped int, err error) {
	hs := domain.GermanHolidays(year, land, loc)
	// Collect first, write as one batch. The previous per-holiday Add
	// loop left partial state on failure (disk-full on holiday 7 of 10
	// kept 6 rows written with no signal which had landed); AddRange
	// already moved to AddBatch for the same reason. All-or-nothing
	// matches the user's mental model: "Sync ist durchgelaufen oder
	// nicht" — kein Halbzustand.
	var toAdd []domain.DayOff
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
		toAdd = append(toAdd, domain.DayOff{Date: h.Date, Kind: h.Kind, Label: h.Label})
	}
	if len(toAdd) == 0 {
		return 0, skipped, nil
	}
	if err := w.Store.AddBatch(toAdd); err != nil {
		return 0, skipped, err
	}
	return len(toAdd), skipped, nil
}
