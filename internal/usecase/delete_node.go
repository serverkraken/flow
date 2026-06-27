package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

// DeleteNode removes a project. Owner-scoped via the store.
type DeleteNode struct {
	Nodes	ports.NodeStore
}

func (uc DeleteNode) Execute(ctx context.Context, ownerID, id string) error {
	return uc.Nodes.Delete(ctx, ownerID, id)
}
