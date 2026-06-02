package flockstate

// TODO(Task12): Remove this file once use cases migrate to the new
// multi-device sqliteclient.ActiveSessions. The Active-state methods below
// are legacy single-device markers; the new ports.ActiveSessionStore lives
// in sqliteclient and is never synced via these file-based paths.

import "time"

// GetActive returns the active-session start time, or nil when no
// session is running.
func (s *State) GetActive() (*time.Time, error) { return readEpoch(s.activePath) }

// SetActive writes the active-session start time.
func (s *State) SetActive(t time.Time) error { return writeEpoch(s.activePath, t) }

// ClearActive removes the active marker. Idempotent: removing a missing
// file is not an error.
func (s *State) ClearActive() error { return removeIfExists(s.activePath) }
