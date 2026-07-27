package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DeleteNodeLogo removes a node's uploaded logo and clears its LogoRef, so
// rendering falls back to icon/glyph. Absent logo is a no-op.
type DeleteNodeLogo struct {
	Nodes     ports.NodeStore
	Logos     ports.NodeLogoStore
	Aggregate ports.NodeAggregateStore
	Clock     ports.Clock
}

func (uc DeleteNodeLogo) Execute(ctx context.Context, ownerID, nodeID string) (domain.Node, error) {
	if uc.Aggregate != nil {
		return uc.Aggregate.UpdateAggregate(ctx, ownerID, nodeID, func(n domain.Node) (domain.Node, ports.NodeAggregateChanges, error) {
			if n.LogoRef != "" {
				n.UpdatedAt = uc.Clock.Now()
			}
			n.LogoRef = ""
			return n, ports.NodeAggregateChanges{Logo: ports.NodeLogoDelete}, nil
		})
	}
	n, err := uc.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	if err := uc.Logos.Delete(ctx, ownerID, nodeID); err != nil {
		return domain.Node{}, err
	}
	if n.LogoRef == "" {
		return n, nil
	}
	n.LogoRef = ""
	n.UpdatedAt = uc.Clock.Now()
	return uc.Nodes.Update(ctx, ownerID, n)
}
