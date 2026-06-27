package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// NodeAncestors returns a node and its ancestors ordered leaf→root.
type NodeAncestors struct {
	Nodes ports.NodeStore
}

func (uc NodeAncestors) Execute(ctx context.Context, ownerID, id string) ([]domain.Node, error) {
	return uc.Nodes.Ancestors(ctx, ownerID, id)
}
