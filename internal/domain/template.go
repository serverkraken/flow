package domain

import "time"

// SessionTemplate is a recurring (start-of-day, duration, tag) shape derived
// from past sessions. Surfaced in the TUI entry form so common patterns
// (standup, daily review) can be inserted in one keystroke.
type SessionTemplate struct {
	Start    time.Duration // offset from midnight, rounded to 15 min
	Duration time.Duration // duration rounded to 15 min
	Tag      string
	Count    int       // how often this exact shape was seen
	Latest   time.Time // most recent occurrence — tiebreaker for sort
}
