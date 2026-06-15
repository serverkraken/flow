package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type GetDocument struct{ Docs ports.DocumentStore }

func (uc GetDocument) Execute(ctx context.Context, ownerID, id string) (domain.Document, error) {
	return uc.Docs.Get(ctx, ownerID, id)
}
