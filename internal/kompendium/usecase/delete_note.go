package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// DeleteNote removes a note from the store.
// Returns ports.ErrNoteNotFound when the ID doesn't exist.
type DeleteNote struct {
	Store ports.NoteStore
}

// NewDeleteNote wires the use case with its required ports.
func NewDeleteNote(store ports.NoteStore) *DeleteNote {
	return &DeleteNote{Store: store}
}

// Execute deletes the note at id from the store.
func (u *DeleteNote) Execute(ctx context.Context, id domain.ID) error {
	if err := u.Store.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete %q: %w", id, err)
	}
	return nil
}
