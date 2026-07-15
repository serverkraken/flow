package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ResolveNode resolves a project from the binding registry or its Git identity
// using the remote slug, machine ID, and current working directory. Explicit
// remote bindings win so they can act as aliases/overrides, followed by the
// canonical origin slug, the normalized upstream URL, and finally path bindings.
type ResolveNode struct {
	Bindings ports.ProjectBindingStore
	Nodes    ports.NodeStore
}

func (uc ResolveNode) Execute(ctx context.Context, ownerID, remoteSlug, machineID, cwd string) (domain.Node, bool, error) {
	bs, err := uc.Bindings.List(ctx, ownerID)
	if err != nil {
		return domain.Node{}, false, err
	}

	if remoteSlug != "" {
		for _, b := range bs {
			if b.Kind != domain.BindingRemote || b.RemoteSlug != remoteSlug {
				continue
			}
			return uc.nodeForBinding(ctx, ownerID, b)
		}

		nodes, err := uc.Nodes.List(ctx, ownerID)
		if err != nil {
			return domain.Node{}, false, err
		}
		var originMatch domain.Node
		originMatches := 0
		for _, node := range nodes {
			if node.Kind != domain.KindRepo || node.OriginSlug != remoteSlug {
				continue
			}
			originMatch = node
			originMatches++
		}
		if originMatches == 1 {
			return originMatch, true, nil
		}
		// An ambiguous canonical origin must not be weakened by picking an
		// arbitrary upstream fallback. A path binding may still disambiguate.
		if originMatches == 0 {
			var match domain.Node
			matches := 0
			for _, node := range nodes {
				if node.Kind != domain.KindRepo {
					continue
				}
				slug, ok := domain.NormalizeRemoteSlug(node.UpstreamGit)
				if !ok || slug != remoteSlug {
					continue
				}
				match = node
				matches++
			}
			if matches == 1 {
				return match, true, nil
			}
		}
	}

	// The remote tiers were handled above; only consider a path binding here.
	b, ok := domain.ResolveBinding(bs, "", machineID, cwd)
	if !ok {
		return domain.Node{}, false, nil
	}
	return uc.nodeForBinding(ctx, ownerID, b)
}

func (uc ResolveNode) nodeForBinding(ctx context.Context, ownerID string, b domain.ProjectBinding) (domain.Node, bool, error) {
	p, err := uc.Nodes.Get(ctx, ownerID, b.NodeID)
	if err != nil {
		return domain.Node{}, false, err
	}
	return p, true, nil
}
