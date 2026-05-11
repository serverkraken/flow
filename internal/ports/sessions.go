package ports

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// SessionStore reads and mutates the sessions log. Implementations persist
// to ~/.tmux/worktime.log as TSV; tests use an in-memory fake.
//
// All methods may be called from inside a Lock.With callback so concurrent
// CLI/TUI writers serialise; readers (LoadAll, LoadFiltered) are
// deliberately unlocked because the TUI's per-second tick would amplify
// contention and the log shape tolerates a stale read between writes.
type SessionStore interface {
	// LoadAll returns every session in the log, oldest first.
	LoadAll() ([]domain.Session, error)
	// LoadFiltered returns sessions for which keep returns true. Equivalent
	// to LoadAll + filter, exposed separately so an adapter can stream
	// large logs without holding everything in memory.
	LoadFiltered(keep func(domain.Session) bool) ([]domain.Session, error)
	// Append writes a single session row. Multi-day spans must be split
	// into one Session per day by the caller (see domain.SplitAtMidnight).
	Append(s domain.Session) error
	// AppendBatch writes several session rows in a single atomic
	// operation. Use cases that produce N parts via SplitAtMidnight
	// (multi-midnight Stop / Pause / Toggle) call this so a partial
	// failure on retry cannot create duplicate rows for the parts that
	// were already persisted before the crash. An empty batch is a
	// no-op and returns nil.
	AppendBatch(sessions []domain.Session) error
	// Rewrite replaces the entire log atomically. Used by Edit/Delete
	// session paths.
	Rewrite(sessions []domain.Session) error
}

// ActiveSessionStore manages the small per-process state markers:
// worktime.state (currently running session start time) and worktime.pause
// (last pause stop time). Both are tiny and read very frequently; the
// implementation should be cheap.
type ActiveSessionStore interface {
	// GetActive returns the start timestamp of the running session, or
	// nil when idle.
	GetActive() (*time.Time, error)
	// SetActive writes a new start timestamp.
	SetActive(t time.Time) error
	// ClearActive removes the marker, marking the session as ended.
	ClearActive() error

	// GetPause returns the stop timestamp of the last pause, or nil when
	// not in pause-mode.
	GetPause() (*time.Time, error)
	// SetPause writes a pause marker.
	SetPause(t time.Time) error
	// ClearPause removes the pause marker.
	ClearPause() error
}

// Lock guards mutations across SessionStore + ActiveSessionStore. The
// adapter implementation typically wraps a flock on ~/.tmux/worktime.lock.
//
// Use cases that mutate state should run their work inside With(fn) so
// concurrent CLI invocations and the TUI cannot race.
type Lock interface {
	With(fn func() error) error
}
