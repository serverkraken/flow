package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

type SetPinned struct{ Docs ports.DocumentStore }

func (uc SetPinned) Execute(ctx context.Context, ownerID, id string, pinned bool) error {
	return uc.Docs.SetPinned(ctx, ownerID, id, pinned)
}
