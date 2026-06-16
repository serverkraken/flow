package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SearchDocuments runs a ranked full-text + fuzzy search over the owner's
// documents, optionally AND-filtered by tags.
type SearchDocuments struct{ Docs ports.DocumentStore }

func (uc SearchDocuments) Execute(ctx context.Context, ownerID, q string, tags []string) ([]domain.SearchHit, error) {
	return uc.Docs.Search(ctx, ownerID, q, tags)
}
