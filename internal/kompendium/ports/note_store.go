package ports

import (
	"context"
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

// ErrNoteNotFound is returned when a NoteStore cannot locate a note by ID.
var ErrNoteNotFound = errors.New("note not found")

// ListFilter narrows the result of NoteStore.List. Empty fields mean "no
// filter on that dimension".
type ListFilter struct {
	Type    domain.NoteType
	Project string
	Limit   int
}

// NoteEntry is a single row returned by NoteStore.List. It carries enough
// metadata to render a list view without loading the full body.
type NoteEntry struct {
	ID    domain.ID
	Meta  domain.Frontmatter
	Mtime time.Time
}

// NoteStore manages note persistence for the notebook. Implementations own
// frontmatter handling and storage layout. Editor flow now uses tempfiles;
// see usecase.EditNote.
type NoteStore interface {
	Get(ctx context.Context, id domain.ID) (domain.Note, error)
	Put(ctx context.Context, note domain.Note) error
	Delete(ctx context.Context, id domain.ID) error
	Exists(ctx context.Context, id domain.ID) (bool, error)
	List(ctx context.Context, filter ListFilter) ([]NoteEntry, error)
}
