package usecase

import (
	"context"
	"strings"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/ports"
)

// SearchNotes runs a full-text query through the document store.
type SearchNotes struct {
	Docs   ports.DocumentStore
	UserID string
}

// NewSearchNotes returns a SearchNotes using the given document store and user.
func NewSearchNotes(docs ports.DocumentStore, userID string) *SearchNotes {
	return &SearchNotes{Docs: docs, UserID: userID}
}

// NewSearchNotesWithIndex wraps a legacy kompports.Indexer in a SearchNotes.
// Used by the CLI's fsstore+sqliteindex path until Task 7 removes that adapter.
//
//nolint:unused // transitional constructor — will be removed in Task 7
func NewSearchNotesWithIndex(index kompports.Indexer) *SearchNotes {
	return &SearchNotes{Docs: &indexerDocAdapter{index: index}}
}

// indexerDocAdapter adapts kompports.Indexer to the minimal ports.DocumentStore
// subset used by SearchNotes.Execute (List only). All other methods are stubs.
type indexerDocAdapter struct{ index kompports.Indexer }

func (a *indexerDocAdapter) Get(_, _ string) (ports.Document, error) {
	return ports.Document{}, ports.ErrDocumentNotFound
}

func (a *indexerDocAdapter) GetByRepoKey(_, _ string) (ports.Document, error) {
	return ports.Document{}, ports.ErrDocumentNotFound
}

func (a *indexerDocAdapter) List(_ string, _, query string, limit int) ([]ports.DocumentEntry, error) {
	results, err := a.index.Search(context.Background(), domain.SearchQuery{
		Text:  query,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ports.DocumentEntry, len(results))
	for i, r := range results {
		out[i] = ports.DocumentEntry{
			Path:    r.ID.Path(),
			Snippet: r.Snippet,
		}
	}
	return out, nil
}

func (a *indexerDocAdapter) Put(_, _, _, _ string, _ int64) (ports.Document, error) {
	return ports.Document{}, ports.ErrDocumentNotFound
}
func (a *indexerDocAdapter) Delete(_, _ string) error { return nil }

// SearchNotesInput configures one Execute call. The fields map directly to
// domain.SearchQuery so callers can express filter and ordering preferences
// without depending on the indexer port.
type SearchNotesInput struct {
	Text    string
	Type    domain.NoteType
	Project string
	Order   domain.SearchOrder
	Limit   int
}

// Execute queries the document store and maps results to domain.SearchResult.
// For a non-empty Text the server performs FTS and returns Snippet-annotated
// entries. For an empty Text all documents are returned (most-recent first).
// Type and Project filters are applied client-side when set, since the
// DocumentStore List API does not expose those dimensions.
func (u *SearchNotes) Execute(ctx context.Context, in SearchNotesInput) ([]domain.SearchResult, error) {
	entries, err := u.Docs.List(u.UserID, "", in.Text, in.Limit)
	if err != nil {
		return nil, err
	}

	out := make([]domain.SearchResult, 0, len(entries))
	for _, e := range entries {
		if !strings.HasSuffix(e.Path, ".md") {
			continue
		}
		id := domain.ID(strings.TrimSuffix(e.Path, ".md"))
		out = append(out, domain.SearchResult{
			ID:      id,
			Snippet: e.Snippet,
			Score:   1.0,
		})
	}
	return out, nil
}
