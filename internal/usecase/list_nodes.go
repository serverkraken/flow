package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListNodes returns the user's projects, ordered by name.
type ListNodes struct {
	Nodes	ports.NodeStore
}

func (uc ListNodes) Execute(ctx context.Context, ownerID string) ([]domain.Node, error) {
	return uc.Nodes.List(ctx, ownerID)
}
