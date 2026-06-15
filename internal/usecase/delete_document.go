package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

type DeleteDocument struct{ Docs ports.DocumentStore }

func (uc DeleteDocument) Execute(ctx context.Context, ownerID, id string) error {
	return uc.Docs.Delete(ctx, ownerID, id)
}
