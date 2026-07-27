package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ResolveEngagement resolves the cwd/remote context to a node, walks its ancestor
// chain, and returns the engagement at its root. Worktime books against this.
type ResolveEngagement struct {
	Resolve ResolveNode
	Nodes   ports.NodeStore
}

func (uc ResolveEngagement) Execute(ctx context.Context, ownerID, remoteSlug, machineID, cwd string) (domain.Node, bool, error) {
	node, ok, err := uc.Resolve.Execute(ctx, ownerID, remoteSlug, machineID, cwd)
	if err != nil || !ok {
		return domain.Node{}, ok, err
	}
	chain, err := uc.Nodes.Ancestors(ctx, ownerID, node.ID)
	if err != nil {
		return domain.Node{}, false, err
	}
	eng, ok := domain.ResolveEngagement(chain)
	if !ok {
		return domain.Node{}, false, nil
	}
	return eng, true, nil
}
