package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListDocumentsPage returns one page of a user's documents plus the total.
type ListDocumentsPage struct{ store ports.DocumentStore }

func NewListDocumentsPage(store ports.DocumentStore) *ListDocumentsPage {
	return &ListDocumentsPage{store: store}
}

func (uc *ListDocumentsPage) Execute(ctx context.Context, ownerID string, nodeID *string, tags []string, limit, offset int) ([]domain.Document, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return uc.store.ListPage(ctx, ownerID, nodeID, limit, offset, tags...)
}
