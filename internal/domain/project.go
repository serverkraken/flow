package domain

import "time"

// Project is a worktime-tracking category — distinct from `SourceDir`
// (file-system project directory). A Session belongs to exactly one
// Project; the TUI picker on `s` sorts Projects MRU-first via LastUsedAt.
//
// Slug is auto-generated from Name (lowercase, ASCII, `-` for spaces) and
// unique per UserID. ArchivedAt is a soft-delete: archived Projects are
// hidden from the picker but their historic Sessions stay intact.
type Project struct {
	ID         string // UUID v4
	UserID     string
	Name       string
	Slug       string
	CreatedAt  time.Time
	LastUsedAt time.Time
	ArchivedAt *time.Time // nil = active
	Version    int64
}
