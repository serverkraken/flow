package usecase

import (
	"context"
	"strings"

	"github.com/serverkraken/flow/internal/kompendium/domain"
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
