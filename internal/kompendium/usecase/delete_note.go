package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// DeleteNote removes a note from the store and drops it from the index.
// Returns ports.ErrNoteNotFound when the ID doesn't exist; index removal
// is best-effort (a stale index is recoverable via `index rebuild`).
type DeleteNote struct {
	Store ports.NoteStore
	Index ports.Indexer // optional
}

// NewDeleteNote wires the use case with its required ports.
func NewDeleteNote(store ports.NoteStore, index ports.Indexer) *DeleteNote {
	return &DeleteNote{Store: store, Index: index}
}

// Execute deletes the note at id from the store, then best-effort removes
// it from the index. The index error is swallowed so a successful
// store-side deletion isn't reported as a failure to the user.
func (u *DeleteNote) Execute(ctx context.Context, id domain.ID) error {
	if err := u.Store.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete %q: %w", id, err)
	}
	if u.Index != nil {
		_ = u.Index.Delete(ctx, id)
	}
	return nil
}
