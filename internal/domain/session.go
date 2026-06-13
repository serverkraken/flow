package domain

import "time"

// Session is a completed work session as logged on disk.
//
// Phase-1 M2 extends the struct with ID (UUID), UserID + ProjectID
// (required for multi-device sync), Version (Lamport per-row from server),
// and UpdatedAt (last mutation timestamp). Legacy callers that build
// Sessions without these fields still compile — fields zero-initialise —
// but the sqliteclient adapter rejects writes with empty UserID/ProjectID.
//
// ProjectName is a transient display field — never persisted. The usecase
// layer populates it via a Projects store join so the TUI can render human-
// readable names without requiring each caller to do its own lookup.
type Session struct {
	ID          string // UUID v4; legacy TSV rows get UUIDv5(date+start+tag+note) during migration
	UserID      string
	ProjectID   string
	ProjectName string // transient: set by WorktimeReader.enrichSessions, empty on raw store load
	Date        time.Time
	Start       time.Time
	Stop        time.Time
	Elapsed     time.Duration
	Tag         string // optional category, e.g. "deep", "meeting"
	Note        string // optional one-line annotation
	Version     int64  // Lamport per row, increments on server-side update
	UpdatedAt   time.Time
}
