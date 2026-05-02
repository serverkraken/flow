package ports

import "time"

// LinkStore persists per-day attachments to Kompendium notes (worktime
// day → list of note IDs). Backed by ~/.tmux/worktime-links.tsv.
type LinkStore interface {
	// ListByDate returns the note IDs attached to date, in insertion
	// order. Empty slice when none.
	ListByDate(date time.Time) ([]string, error)
	// Add attaches noteID to date. Idempotent — adding an existing pair
	// is a no-op.
	Add(date time.Time, noteID string) error
	// Remove detaches noteID from date. Removing a non-existent pair is
	// a no-op.
	Remove(date time.Time, noteID string) error
}
