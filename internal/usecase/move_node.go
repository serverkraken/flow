package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ErrNodeCycle is returned when a move would make a node its own ancestor.
var ErrNodeCycle = errors.New("usecase: move would create a cycle")

// MoveNode reparents a node, enforcing kind rules and acyclicity.
type MoveNode struct {
	Nodes ports.NodeStore
}

func (uc MoveNode) Execute(ctx context.Context, ownerID, id string, newParentID *string) (domain.Node, error) {
	node, err := uc.Nodes.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Node{}, err
	}
	if newParentID == nil {
		if node.Kind != domain.KindEngagement {
			return domain.Node{}, fmt.Errorf("%w: only engagements may be roots", domain.ErrInvalidNode)
		}
		return uc.Nodes.Reparent(ctx, ownerID, id, nil)
	}
	parent, err := uc.Nodes.Get(ctx, ownerID, *newParentID)
	if err != nil {
		return domain.Node{}, err
	}
	// Cycle check first: the new parent's ancestor chain (which includes the new
	// parent itself) must not contain the node being moved.
	chain, err := uc.Nodes.Ancestors(ctx, ownerID, *newParentID)
	if err != nil {
		return domain.Node{}, err
	}
	for _, a := range chain {
		if a.ID == id {
			return domain.Node{}, ErrNodeCycle
		}
	}
	if !domain.AllowedChildKind(parent.Kind, node.Kind) {
		return domain.Node{}, fmt.Errorf("%w: %s cannot be a child of %s", domain.ErrInvalidNode, node.Kind, parent.Kind)
	}
	return uc.Nodes.Reparent(ctx, ownerID, id, newParentID)
}
