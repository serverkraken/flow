package jsonflowstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// Store reads and writes flow's UI state plus the one-shot deep-link
// marker written by goto.sh.
type Store struct {
	statePath      string
	nextScreenPath string
}

// New constructs a Store backed by the given file paths. Both files'
// parent directory is created on demand by Save / WriteNextScreen.
func New(statePath, nextScreenPath string) *Store {
	return &Store{statePath: statePath, nextScreenPath: nextScreenPath}
}

// Load returns the persisted state, falling back to DefaultFlowState
// when the file is absent or malformed.
func (s *Store) Load() (domain.FlowState, error) {
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		return domain.DefaultFlowState(), nil
	}
	var st domain.FlowState
	if err := json.Unmarshal(data, &st); err != nil {
		return domain.DefaultFlowState(), nil
	}
	if !domain.IsValidScreen(st.Screen) {
		st.Screen = domain.ScreenPalette
	}
	return st, nil
}

// Save persists the state, creating the parent directory when needed.
func (s *Store) Save(st domain.FlowState) error {
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return writeFileAtomic(s.statePath, data, 0o644)
}

// ConsumeNextScreen reads and removes the one-shot deep-link marker.
// Returns "" when the file is missing or its content is not a valid
// screen identifier.
func (s *Store) ConsumeNextScreen() (string, error) {
	data, err := os.ReadFile(s.nextScreenPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	// Best-effort cleanup: a remove failure here doesn't prevent the
	// caller from using the deep-link content; failure to clean up
	// just means the link could fire twice, which is harmless.
	_ = os.Remove(s.nextScreenPath)
	screen := strings.TrimSpace(string(data))
	if !domain.IsValidScreen(screen) {
		return "", nil
	}
	return screen, nil
}

// WriteNextScreen records a one-shot deep-link to screen.
func (s *Store) WriteNextScreen(screen string) error {
	if !domain.IsValidScreen(screen) {
		return errors.New("unknown screen: " + screen)
	}
	if err := os.MkdirAll(filepath.Dir(s.nextScreenPath), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(s.nextScreenPath, []byte(screen), 0o644)
}

// writeFileAtomic writes data via temp+fsync+rename so a crash mid-write
// can never leave a truncated file. Without fsync the new content can
// land in the page cache after the rename has already updated the
// directory entry, which on power loss surfaces as a zero-length file.
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
