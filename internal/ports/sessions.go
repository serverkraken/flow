package ports

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// SessionStore reads and mutates the sessions log. The new interface is
// ID-based (Upsert/Delete) and user-scoped (userID param). Implementations
// persist to SQLite (sqliteclient); the tsvsessions adapter satisfies this
// interface via a shim until Task 19 removes it.
//
// All methods may be called from inside a Lock.With callback so concurrent
// CLI/TUI writers serialise; Load/LoadFiltered are deliberately unlocked
// because the TUI's per-second tick would amplify contention and the log
// shape tolerates a stale read between writes.
type SessionStore interface {
	// Load returns all sessions for the user, ordered by Date ASC, Start ASC.
	Load(userID string) ([]domain.Session, error)

	// LoadFiltered returns sessions for which keep returns true.
	LoadFiltered(userID string, keep func(domain.Session) bool) ([]domain.Session, error)

	// Upsert inserts or updates a single Session by ID.
	Upsert(s domain.Session) error

	// UpsertBatch is the multi-row form used by sync ingestion.
	UpsertBatch(sessions []domain.Session) error

	// Delete removes by ID.
	Delete(userID, id string) error

	// Append writes a single session row. Legacy path; replaced by Upsert
	// in Task 10. Deleted together with the tsvsessions adapter in Task 19.
	Append(s domain.Session) error

	// AppendBatch writes multiple session rows atomically. Legacy path;
	// replaced by UpsertBatch in Task 10. Deleted in Task 19.
	AppendBatch(sessions []domain.Session) error

	// Rewrite replaces the entire log atomically. Legacy path; the
	// ID-based interface has no direct equivalent. Callers will be
	// refactored in Task 10. Deleted in Task 19.
	Rewrite(sessions []domain.Session) error
}

// ErrSessionVersionConflict is returned by SessionStore.Upsert when the
// server version differs from the client's expected version (OCC reject).
var ErrSessionVersionConflict = errSentinel("flow: session version conflict")

// LegacyActiveStore manages the small per-process state markers:
// worktime.state (currently running session start time) and worktime.pause
// (last pause stop time). Both are tiny and read very frequently; the
// implementation should be cheap.
//
// Renamed from ActiveSessionStore; scheduled for removal in Task 9 when
// the jsonflowstate reduction lands. Callers in usecase/adapter still
// use this under the new name until then.
type LegacyActiveStore interface {
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

// Lock guards mutations across SessionStore + LegacyActiveStore. The
// adapter implementation typically wraps a flock on ~/.tmux/worktime.lock.
//
// Use cases that mutate state should run their work inside With(fn) so
// concurrent CLI invocations and the TUI cannot race.
type Lock interface {
	With(fn func() error) error
}
