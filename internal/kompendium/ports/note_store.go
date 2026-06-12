package ports

import (
	"context"
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

// ErrNoteNotFound is returned when a NoteStore cannot locate a note by ID.
var ErrNoteNotFound = errors.New("note not found")

// ErrVersionConflict is returned by NoteStore.Put when the server-side
// version does not match the expected version (optimistic-concurrency
// conflict). It wraps the underlying transport-layer conflict error so
// callers inside the kompendium subtree do not need to import
// internal/ports directly.
var ErrVersionConflict = errors.New("note version conflict")

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

// RawSearchEntry is a body-less document entry returned by NoteSearcher.
// It uses only stdlib types so the interface can live in kompendium/ports
// without importing internal/ports.
type RawSearchEntry struct {
	// Path is the document path relative to the store root (e.g. "daily/2026-04-25.md").
	Path string
	// Snippet is the FTS-highlighted excerpt when the server performs full-text
	// search; empty for list-all queries.
	Snippet string
}

// NoteSearcher is the search-side dependency for SearchNotes. It exposes
// the raw list/search operation in terms that kompendium/ports can express
// without importing internal/ports. Implementations: apistore.Store and
// testutil.FakeDocStore (via ListRaw).
type NoteSearcher interface {
	ListRaw(ctx context.Context, userID, prefix, query string, limit int) ([]RawSearchEntry, error)
}
