package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListArtifacts returns artifact meta (no bytes) reachable from nodeID: the
// node itself plus its ancestor chain (NOT its subtree) — the same scope a
// document's ![[slug]] resolves against, nearest node first.
type ListArtifacts struct {
	Nodes     ports.NodeStore
	Artifacts ports.ArtifactStore
}

func (uc ListArtifacts) Execute(ctx context.Context, ownerID, nodeID string) ([]domain.Artifact, error) {
	chain, err := uc.Nodes.Ancestors(ctx, ownerID, nodeID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(chain))
	for i, n := range chain {
		ids[i] = n.ID
	}
	return uc.Artifacts.List(ctx, ownerID, ids...)
}
