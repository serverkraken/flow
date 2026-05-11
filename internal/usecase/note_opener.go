package usecase

import (
	"errors"

	"github.com/serverkraken/flow/internal/ports"
)

// NoteOpener launches a Kompendium note in the user's editor via the
// NoteLauncher port. The use case adds the empty-id guard so every
// adapter doesn't need to repeat it. Read-only view happens in-process
// via the integrated markdown renderer (Heute `o`-Key), not through
// this port.
type NoteOpener struct {
	Launcher ports.NoteLauncher
}

// Open launches the note in an editor (typically tmux split + nvim).
func (o *NoteOpener) Open(id string) error {
	if id == "" {
		return errors.New("note id darf nicht leer sein")
	}
	return o.Launcher.Open(id)
}
