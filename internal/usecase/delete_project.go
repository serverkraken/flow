package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

// DeleteProject removes a project. Owner-scoped via the store.
type DeleteProject struct {
	Projects ports.ProjectStore
}

func (uc DeleteProject) Execute(ctx context.Context, ownerID, id string) error {
	return uc.Projects.Delete(ctx, ownerID, id)
}
