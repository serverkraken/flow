// Package state manages flow's persistent JSON state and the one-shot
// next-screen file written by goto.sh for deep-linking.
package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Screen identifiers — must match goto.sh arguments.
const (
	Palette    = "palette"
	Projects   = "projects"
	Worktime   = "worktime"
	Cheatsheet = "cheatsheet"
)

// State holds the persisted UI state restored on next launch.
type State struct {
	Screen string `json:"screen"`
	Filter string `json:"filter"`
	Cursor int    `json:"cursor"`
}

// Default returns a fresh state with the palette as the active screen.
func Default() State { return State{Screen: Palette} }

// Load reads ~/.cache/flow/state.json. Returns Default() if the file is absent
// or malformed (non-fatal — first launch is normal).
func Load() State {
	data, err := os.ReadFile(stateFile())
	if err != nil {
		return Default()
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return Default()
	}
	if !validScreen(s.Screen) {
		s.Screen = Palette
	}
	return s
}

// Save writes s to ~/.cache/flow/state.json, creating the directory as needed.
func Save(s State) error {
	dir := cacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile(), data, 0o644)
}

// CheckNextScreen returns the screen name written by goto.sh and removes the
// file so it fires only once. Returns "" when no deep-link is pending.
func CheckNextScreen() string {
	path := filepath.Join(cacheDir(), "next-screen")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	_ = os.Remove(path)
	screen := strings.TrimSpace(string(data))
	if !validScreen(screen) {
		return ""
	}
	return screen
}

// WriteNextScreen writes screen to ~/.cache/flow/next-screen (used by goto.sh).
func WriteNextScreen(screen string) error {
	if !validScreen(screen) {
		return errors.New("unknown screen: " + screen)
	}
	dir := cacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "next-screen"), []byte(screen), 0o644)
}

func cacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "flow")
}

func stateFile() string { return filepath.Join(cacheDir(), "state.json") }

func validScreen(s string) bool {
	switch s {
	case Palette, Projects, Worktime, Cheatsheet:
		return true
	}
	return false
}
