package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// Open opens an existing note in the editor. It does not create a missing
// note — that is the job of one of the Create* use cases.
type Open struct {
	Store  ports.NoteStore
	Editor ports.Editor
}

// NewOpen wires the use case with its required ports.
func NewOpen(store ports.NoteStore, editor ports.Editor) *Open {
	return &Open{Store: store, Editor: editor}
}

// OpenInput carries the note ID to open.
type OpenInput struct {
	ID domain.ID
}

// Execute checks that the note exists and asks the editor to open it. A
// missing note returns ports.ErrNoteNotFound so callers can show a helpful
// message rather than spawning an editor on a phantom path.
func (u *Open) Execute(ctx context.Context, in OpenInput) error {
	exists, err := u.Store.Exists(ctx, in.ID)
	if err != nil {
		return fmt.Errorf("exists: %w", err)
	}
	if !exists {
		return ports.ErrNoteNotFound
	}
	edit := EditNote{Store: u.Store, Editor: u.Editor}
	if err := edit.Execute(ctx, in.ID); err != nil {
		return err
	}
	return nil
}
