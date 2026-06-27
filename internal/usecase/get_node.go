package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// GetNode fetches a single owner-scoped project by id.
type GetNode struct {
	Nodes	ports.NodeStore
}

func (uc GetNode) Execute(ctx context.Context, ownerID, id string) (domain.Node, error) {
	return uc.Nodes.Get(ctx, ownerID, id)
}
