package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// SearchNotes runs a full-text query through the indexer.
type SearchNotes struct {
	Index ports.Indexer
}

// NewSearchNotes returns a SearchNotes using the given indexer.
func NewSearchNotes(index ports.Indexer) *SearchNotes {
	return &SearchNotes{Index: index}
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

// Execute forwards the query to the indexer.
func (u *SearchNotes) Execute(ctx context.Context, in SearchNotesInput) ([]domain.SearchResult, error) {
	return u.Index.Search(ctx, domain.SearchQuery{
		Text:    in.Text,
		Type:    in.Type,
		Project: in.Project,
		Order:   in.Order,
		Limit:   in.Limit,
	})
}
