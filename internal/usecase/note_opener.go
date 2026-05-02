package usecase

import (
	"errors"

	"github.com/serverkraken/flow/internal/ports"
)

// NoteOpener launches a Kompendium note in the user's environment via
// the NoteLauncher port. The use case adds the empty-id guard so every
// adapter doesn't need to repeat it.
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

// View launches the note in a read-only viewer (typically tmux split + glow).
func (o *NoteOpener) View(id string) error {
	if id == "" {
		return errors.New("note id darf nicht leer sein")
	}
	return o.Launcher.View(id)
}
