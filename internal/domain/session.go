package domain

import "time"

// Session is a completed work session as logged on disk.
type Session struct {
	Date    time.Time
	Start   time.Time
	Stop    time.Time
	Elapsed time.Duration
	Tag     string // optional category, e.g. "deep", "meeting"
	Note    string // optional one-line annotation
}
