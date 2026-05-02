package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// CreateDaily creates today's daily note (if missing) and opens it in the
// editor. Idempotent: calling it twice the same day reuses the existing note.
//
// Index is optional. When set, the note is re-read after the editor closes
// and upserted into the FTS5 index so `kompendium search` finds the new
// content immediately.
type CreateDaily struct {
	Store  ports.NoteStore
	Clock  ports.Clock
	Editor ports.Editor
	Index  ports.Indexer
}

// NewCreateDaily wires the use case with its required ports.
func NewCreateDaily(store ports.NoteStore, clock ports.Clock, editor ports.Editor) *CreateDaily {
	return &CreateDaily{Store: store, Clock: clock, Editor: editor}
}

// CreateDailyOutput reports the resolved ID, whether the note was newly
// created, and the filesystem path passed to the editor.
type CreateDailyOutput struct {
	ID      domain.ID
	Created bool
	Path    string
}

// Execute resolves today's daily ID, ensures the note exists (creating it
// with default frontmatter if needed), and asks the editor to open it.
func (u *CreateDaily) Execute(ctx context.Context) (CreateDailyOutput, error) {
	date := u.Clock.Now().UTC().Format("2006-01-02")
	id := domain.ID("daily/" + date)

	exists, err := u.Store.Exists(ctx, id)
	if err != nil {
		return CreateDailyOutput{}, fmt.Errorf("exists: %w", err)
	}

	if !exists {
		note, err := buildDailyTemplate(id, date)
		if err != nil {
			return CreateDailyOutput{}, err
		}
		if err := u.Store.Put(ctx, note); err != nil {
			return CreateDailyOutput{}, fmt.Errorf("put: %w", err)
		}
	}

	path := u.Store.Path(id)
	if err := u.Editor.Edit(ctx, path); err != nil {
		return CreateDailyOutput{}, fmt.Errorf("edit: %w", err)
	}
	reindex(ctx, u.Store, u.Index, id)
	return CreateDailyOutput{ID: id, Created: !exists, Path: path}, nil
}

func buildDailyTemplate(id domain.ID, date string) (domain.Note, error) {
	return domain.NewNote(id, domain.Frontmatter{
		ID:   id.String(),
		Type: domain.TypeDaily,
		Date: date,
	}, []byte{})
}
