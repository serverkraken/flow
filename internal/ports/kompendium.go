package ports

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// KompendiumGateway provides read access to the Kompendium notebook.
// Adapters typically shell out to the `kompendium` binary; in F-future
// this will become a direct library call once kompendium ships in-tree.
type KompendiumGateway interface {
	// DailyExists reports whether a daily note for date is present on disk.
	// Best-effort: returns false when kompendium is unavailable.
	DailyExists(date time.Time) bool
	// List returns every known note via `kompendium ls --json`.
	List() ([]domain.KompendiumNote, error)
	// ResolvePath returns the filesystem path of a note ID, or "" when
	// the ID can't be resolved.
	ResolvePath(id string) (string, error)
}

// NoteLauncher opens a Kompendium note in the user's environment. Open()
// uses an editor (typically tmux split + nvim); View() is read-only
// (typically tmux split + glow).
type NoteLauncher interface {
	Open(id string) error
	View(id string) error
}
