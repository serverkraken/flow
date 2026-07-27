package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type ListDocuments struct{ Docs ports.DocumentStore }

func (uc ListDocuments) Execute(ctx context.Context, ownerID string, nodeID *string, tags []string) ([]domain.Document, error) {
	return uc.Docs.List(ctx, ownerID, nodeID, tags...)
}
