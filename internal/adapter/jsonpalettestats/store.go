package jsonpalettestats

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/serverkraken/flow/internal/adapter/atomicfile"
	"github.com/serverkraken/flow/internal/domain"
)

// Store reads and writes palette stats to a JSON file.
type Store struct {
	path string
}

// New constructs a Store at path. The parent directory is created on
// first save.
func New(path string) *Store {
	return &Store{path: path}
}

// Load returns the persisted stats. Missing file → zero-value stats
// with no error. A malformed file IS surfaced as an error so the
// caller can decide whether to wipe and start over.
func (s *Store) Load() (domain.PaletteStats, error) {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{}}, nil
	}
	if err != nil {
		return domain.PaletteStats{}, err
	}
	defer f.Close() //nolint:errcheck

	var actions map[string]domain.PaletteActionStat
	if err := json.NewDecoder(f).Decode(&actions); err != nil {
		return domain.PaletteStats{}, err
	}
	if actions == nil {
		actions = map[string]domain.PaletteActionStat{}
	}
	return domain.PaletteStats{Actions: actions}, nil
}

// Save persists the stats. The on-disk JSON is the bare actions map,
// matching the layout the legacy implementation has been writing —
// so users upgrading don't lose history.
//
// Crash safety: write goes through temp+fsync+rename so a power loss
// mid-write cannot leave a truncated file that the next Load would
// silently treat as empty stats.
func (s *Store) Save(stats domain.PaletteStats) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	actions := stats.Actions
	if actions == nil {
		actions = map[string]domain.PaletteActionStat{}
	}
	data, err := json.Marshal(actions)
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(s.path, data, 0o644)
}
