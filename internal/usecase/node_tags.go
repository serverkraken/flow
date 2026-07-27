package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// NodeTags reads and replaces tags only after verifying that the node belongs
// to the caller. This prevents orphan taggings and cross-owner node references.
type NodeTags struct {
	Nodes ports.NodeStore
	Tags  ports.TagStore
}

func (uc NodeTags) Get(ctx context.Context, ownerID, nodeID string) ([]domain.Tag, error) {
	if _, err := uc.Nodes.Get(ctx, ownerID, nodeID); err != nil {
		return nil, err
	}
	return uc.Tags.TagsFor(ctx, ownerID, domain.TaggableNode, nodeID)
}

func (uc NodeTags) Set(ctx context.Context, ownerID, nodeID string, raw []string) ([]domain.Tag, error) {
	if _, err := uc.Nodes.Get(ctx, ownerID, nodeID); err != nil {
		return nil, err
	}
	return uc.Tags.SetTags(ctx, ownerID, domain.TaggableNode, nodeID, raw)
}
