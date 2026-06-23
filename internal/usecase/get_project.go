package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// GetProject fetches a single owner-scoped project by id.
type GetProject struct {
	Projects ports.ProjectStore
}

func (uc GetProject) Execute(ctx context.Context, ownerID, id string) (domain.Project, error) {
	return uc.Projects.Get(ctx, ownerID, id)
}
