package ports

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

// IndexEntry is the input shape for a full index rebuild — the notebook's
// current state as a list of (note, mtime) pairs.
type IndexEntry struct {
	Note  domain.Note
	Mtime time.Time
}

// Indexer maintains a search index over the notebook so use cases can answer
// queries without scanning every file. Implementations own the index storage
// (sqlite/FTS5 in production, in-memory in tests).
//
// BacklinksOf and LinksFrom return enriched (ID, Title) pairs so the
// read view can render references without a second per-link store
// fetch. A backlink whose target was deleted has Title="".
type Indexer interface {
	Upsert(ctx context.Context, note domain.Note, mtime time.Time) error
	Delete(ctx context.Context, id domain.ID) error
	Search(ctx context.Context, q domain.SearchQuery) ([]domain.SearchResult, error)
	BacklinksOf(ctx context.Context, id domain.ID) ([]domain.LinkRef, error)
	LinksFrom(ctx context.Context, id domain.ID) ([]domain.LinkRef, error)
	Rebuild(ctx context.Context, all []IndexEntry) error
}
