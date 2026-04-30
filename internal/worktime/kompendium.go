package worktime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// KompendiumNote is one entry returned by `kompendium ls --json`.
type KompendiumNote struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Date    string `json:"date,omitempty"`
	Project string `json:"project,omitempty"`
	MTime   string `json:"mtime,omitempty"`
}

// kompendiumBin is the executable name; overridable in tests via KOMPENDIUM_BIN.
func kompendiumBin() string {
	if v := os.Getenv("KOMPENDIUM_BIN"); v != "" {
		return v
	}
	return "kompendium"
}

// DailyNoteID returns the canonical Kompendium ID for the daily note of a given date.
func DailyNoteID(date time.Time) string {
	return "daily/" + date.Format("2006-01-02")
}

// DailyExists reports whether a daily note for the given date exists on disk.
// Fails silently (returns false) when kompendium is unavailable.
func DailyExists(date time.Time) bool {
	out, err := exec.Command(kompendiumBin(), "path", DailyNoteID(date)).Output()
	if err != nil {
		return false
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// ListKompendiumNotes returns all known Kompendium notes via `kompendium ls --json`.
func ListKompendiumNotes() ([]KompendiumNote, error) {
	out, err := exec.Command(kompendiumBin(), "ls", "--json").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("kompendium ls: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("kompendium ls: %w", err)
	}
	if len(out) == 0 || strings.TrimSpace(string(out)) == "" {
		return nil, nil
	}
	var notes []KompendiumNote
	if err := json.Unmarshal(out, &notes); err != nil {
		return nil, fmt.Errorf("kompendium ls: parse: %w", err)
	}
	return notes, nil
}

// OpenNote opens a Kompendium note in the editor in a horizontal tmux split.
func OpenNote(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("note id darf nicht leer sein")
	}
	return exec.Command("tmux", "split-window", "-h", kompendiumBin(), "open", id).Run()
}

// ViewNote opens a Kompendium note read-only via glow in a horizontal tmux split.
// Resolves the filesystem path through `kompendium path` so glow gets the actual
// markdown file, not a note ID.
func ViewNote(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("note id darf nicht leer sein")
	}
	out, err := exec.Command(kompendiumBin(), "path", id).Output()
	if err != nil {
		return fmt.Errorf("kompendium path: %w", err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return errors.New("note path nicht auflösbar")
	}
	viewer := os.Getenv("FLOW_NOTE_VIEWER")
	if viewer == "" {
		viewer = "glow"
	}
	return exec.Command("tmux", "split-window", "-h", viewer, path).Run()
}

// HumanizeNoteID returns a short, human-friendly label derived from a note ID alone.
// Useful when the full KompendiumNote isn't loaded (e.g. when rendering attached IDs).
func HumanizeNoteID(id string) string {
	switch {
	case strings.HasPrefix(id, "daily/"):
		return "Daily " + strings.TrimPrefix(id, "daily/")
	case strings.HasPrefix(id, "projects/"):
		return "Projekt " + strings.TrimPrefix(id, "projects/")
	case strings.HasPrefix(id, "notes/"):
		return "Notiz " + strings.TrimPrefix(id, "notes/")
	}
	return id
}
