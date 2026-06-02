package flockstate

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/atomicfile"
)

// State implements ports.PauseStore (and ports.LegacyActiveStore via
// legacy_active.go) by storing one Unix epoch per file: the active-session
// start (worktime.state) and the last-pause stop time (worktime.pause). A
// missing file means "not set".
type State struct {
	activePath string
	pausePath  string
}

// NewState constructs a State backed by the given active and pause file
// paths. Parent directories are created on demand on first write.
func NewState(activePath, pausePath string) *State {
	return &State{activePath: activePath, pausePath: pausePath}
}

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
	return atomicfile.WriteFile(path, []byte(strconv.FormatInt(t.Unix(), 10)), 0o644)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
