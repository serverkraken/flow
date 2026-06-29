package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

// SetArchived archives or un-archives a document.
type SetArchived struct{ Docs ports.DocumentStore }

func (uc SetArchived) Execute(ctx context.Context, ownerID, id string, archived bool) error {
	return uc.Docs.SetArchived(ctx, ownerID, id, archived)
}
