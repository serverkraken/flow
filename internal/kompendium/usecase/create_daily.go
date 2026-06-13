package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// CreateDaily creates today's daily note (if missing) and opens it in the
// editor. Idempotent: calling it twice the same day reuses the existing note.
type CreateDaily struct {
	Store  ports.NoteStore
	Clock  ports.Clock
	Editor ports.Editor
}

// NewCreateDaily wires the use case with its required ports.
func NewCreateDaily(store ports.NoteStore, clock ports.Clock, editor ports.Editor) *CreateDaily {
	return &CreateDaily{Store: store, Clock: clock, Editor: editor}
}

// CreateDailyOutput reports the resolved ID and whether the note was newly
// created.
type CreateDailyOutput struct {
	ID      domain.ID
	Created bool
}

// Execute resolves today's daily ID, ensures the note exists (creating it
// with default frontmatter if needed), and asks the editor to open it via a
// tempfile (see EditNote).
func (u *CreateDaily) Execute(ctx context.Context) (CreateDailyOutput, error) {
	// Use the wallclock date in the user's local TZ — daily notes are a
	// human-day concept, not a UTC-day concept. Without this, a Berlin
	// user creating a note at 01:30 CEST would land in yesterday's daily
	// (UTC 23:30 of the previous day).
	date := u.Clock.Now().Format("2006-01-02")
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

	edit := EditNote{Store: u.Store, Editor: u.Editor}
	if err := edit.Execute(ctx, id); err != nil {
		return CreateDailyOutput{}, fmt.Errorf("edit: %w", err)
	}
	return CreateDailyOutput{ID: id, Created: !exists}, nil
}

func buildDailyTemplate(id domain.ID, date string) (domain.Note, error) {
	return domain.NewNote(id, domain.Frontmatter{
		ID:   id.String(),
		Type: domain.TypeDaily,
		Date: date,
	}, []byte{})
}
