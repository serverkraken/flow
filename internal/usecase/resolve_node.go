package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ResolveNode resolves a project from the binding registry using the
// remote slug, machine ID, and current working directory.
type ResolveNode struct {
	Bindings ports.ProjectBindingStore
	Nodes	ports.NodeStore
}

func (uc ResolveNode) Execute(ctx context.Context, ownerID, remoteSlug, machineID, cwd string) (domain.Node, bool, error) {
	bs, err := uc.Bindings.List(ctx, ownerID)
	if err != nil {
		return domain.Node{}, false, err
	}
	b, ok := domain.ResolveBinding(bs, remoteSlug, machineID, cwd)
	if !ok {
		return domain.Node{}, false, nil
	}
	p, err := uc.Nodes.Get(ctx, ownerID, b.NodeID)
	if err != nil {
		return domain.Node{}, false, err
	}
	return p, true, nil
}
