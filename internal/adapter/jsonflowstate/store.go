package jsonflowstate

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/atomicfile"
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

// Load returns the persisted state. A missing file returns the default
// (first launch is normal). A malformed file is also defaulted, so a
// hand-edit gone wrong doesn't lock the user out of the TUI. Other I/O
// errors (permission denied, EIO) are surfaced — silently defaulting on
// those would mask real problems and the next Save would overwrite a
// possibly-recoverable file.
func (s *Store) Load() (domain.FlowState, error) {
	data, err := os.ReadFile(s.statePath)
	if errors.Is(err, fs.ErrNotExist) {
		return domain.DefaultFlowState(), nil
	}
	if err != nil {
		return domain.FlowState{}, err
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
	return atomicfile.WriteFile(s.statePath, data, 0o644)
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
	return atomicfile.WriteFile(s.nextScreenPath, []byte(screen), 0o644)
}
