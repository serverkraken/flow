package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SetCountsTowardTarget always sets a node's Work/Privat override (nil = inherit,
// *true = Work, *false = Privat). Unlike UpdateNodeInput (nil = preserve), this
// applies the value verbatim — the WebUI tri-state control uses it so "set to
// inherit" is expressible. Mirrors SetNodeRate's always-apply shape.
type SetCountsTowardTarget struct {
	Nodes     ports.NodeStore
	Aggregate ports.NodeAggregateStore
	Clock     ports.Clock
}

func (uc SetCountsTowardTarget) Execute(ctx context.Context, ownerID, id string, mode *bool) (domain.Node, error) {
	if uc.Aggregate != nil {
		return uc.Aggregate.UpdateAggregate(ctx, ownerID, id, func(n domain.Node) (domain.Node, ports.NodeAggregateChanges, error) {
			n.CountsTowardTarget = mode
			n.UpdatedAt = uc.Clock.Now()
			return n, ports.NodeAggregateChanges{}, nil
		})
	}
	n, err := uc.Nodes.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Node{}, err // ErrNodeNotFound bubbles to a 404
	}
	n.CountsTowardTarget = mode
	n.UpdatedAt = uc.Clock.Now()
	return uc.Nodes.Update(ctx, ownerID, n)
}
