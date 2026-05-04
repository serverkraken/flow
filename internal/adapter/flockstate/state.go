package flockstate

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// State implements ports.ActiveSessionStore by storing one Unix epoch per
// file: the active-session start (worktime.state) and the last-pause
// stop time (worktime.pause). A missing file means "not set".
type State struct {
	activePath string
	pausePath  string
}

// NewState constructs a State backed by the given active and pause file
// paths. Parent directories are created on demand on first write.
func NewState(activePath, pausePath string) *State {
	return &State{activePath: activePath, pausePath: pausePath}
}

// GetActive returns the active-session start time, or nil when no
// session is running.
func (s *State) GetActive() (*time.Time, error) { return readEpoch(s.activePath) }

// SetActive writes the active-session start time.
func (s *State) SetActive(t time.Time) error { return writeEpoch(s.activePath, t) }

// ClearActive removes the active marker. Idempotent: removing a missing
// file is not an error.
func (s *State) ClearActive() error { return removeIfExists(s.activePath) }

// GetPause returns the pause start time, or nil when not paused.
func (s *State) GetPause() (*time.Time, error) { return readEpoch(s.pausePath) }

// SetPause writes the pause start time.
func (s *State) SetPause(t time.Time) error { return writeEpoch(s.pausePath, t) }

// ClearPause removes the pause marker. Idempotent.
func (s *State) ClearPause() error { return removeIfExists(s.pausePath) }

func readEpoch(path string) (*time.Time, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	epoch, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return nil, err
	}
	t := time.Unix(epoch, 0)
	return &t, nil
}

func writeEpoch(path string, t time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(strconv.FormatInt(t.Unix(), 10)), 0o644)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// writeFileAtomic writes data via temp+fsync+rename so a crash mid-write
// can never leave a truncated or half-written file in place. Without
// fsync the new content can land in the page cache after the rename has
// already updated the directory entry, which on power loss surfaces as
// a zero-length file the next time it's opened.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
