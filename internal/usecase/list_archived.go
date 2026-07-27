package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListArchived returns the owner's archived documents, newest archived_at first.
type ListArchived struct{ Docs ports.DocumentStore }

func (uc ListArchived) Execute(ctx context.Context, ownerID string) ([]domain.Document, error) {
	return uc.Docs.ListArchived(ctx, ownerID)
}
