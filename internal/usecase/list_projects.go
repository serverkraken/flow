package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListProjects returns the user's projects, ordered by name.
type ListProjects struct {
	Projects ports.ProjectStore
}

func (uc ListProjects) Execute(ctx context.Context, ownerID string) ([]domain.Project, error) {
	return uc.Projects.List(ctx, ownerID)
}
