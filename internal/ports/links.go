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
	// CountsByDate returns the number of notes attached per day, keyed
	// by the YYYY-MM-DD date string. Days without attachments are
	// omitted from the map. Used by surfaces that want to flag „dieser
	// Tag hat Notes" without paying N file-reads (one per day) for what
	// could be a single scan.
	CountsByDate() (map[string]int, error)
}
