package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UploadNodeBanner stores a node's banner image (replace-on-upload) and stamps
// the node's BannerRef with the content hash for cache-busting URLs and ETags.
// Mirrors UploadNodeLogo, including its aggregate path: blob and ref land in
// ONE transaction, so a concurrent metadata write cannot lose either.
//
// The WebUI form does not use this — there the banner rides UpdateNodeInput.
// This is the agent-facing path (PUT /api/v1/nodes/{id}/banner, flow-mcp),
// where the banner is the whole request.
type UploadNodeBanner struct {
	Nodes     ports.NodeStore
	Banners   ports.NodeBannerStore
	Aggregate ports.NodeAggregateStore
	Clock     ports.Clock
}

func (uc UploadNodeBanner) Execute(ctx context.Context, ownerID, nodeID string, data []byte) (domain.Node, error) {
	banner, err := buildNodeBanner(ownerID, nodeID, data, uc.Clock.Now())
	if err != nil {
		return domain.Node{}, err
	}
	ref := banner.Ref
	now := banner.UpdatedAt
	if uc.Aggregate != nil {
		return uc.Aggregate.UpdateAggregate(ctx, ownerID, nodeID, func(n domain.Node) (domain.Node, ports.NodeAggregateChanges, error) {
			n.BannerRef = ref
			n.UpdatedAt = now
			return n, ports.NodeAggregateChanges{Banner: ports.NodeBannerPut, BannerValue: banner}, nil
		})
	}
	n, err := uc.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	if err := uc.Banners.Put(ctx, banner); err != nil {
		return domain.Node{}, err
	}
	n.BannerRef = ref
	n.UpdatedAt = now
	return uc.Nodes.Update(ctx, ownerID, n)
}
