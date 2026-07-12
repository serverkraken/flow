package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListArtifacts returns artifact meta (no bytes) reachable from nodeID: the
// node itself plus its ancestor chain (NOT its subtree) — the same scope a
// document's ![[slug]] resolves against, nearest node first — PLUS the
// owner's free (node-less) artifacts appended last (free-artifacts Task 3,
// Spec E1: free ranks root-lowest, below every chain node). nodeID=="" means
// "free only": ListFree(owner) alone, Ancestors is never called.
type ListArtifacts struct {
	Nodes     ports.NodeStore
	Artifacts ports.ArtifactStore
}

func (uc ListArtifacts) Execute(ctx context.Context, ownerID, nodeID string) ([]domain.Artifact, error) {
	if nodeID == "" {
		return uc.Artifacts.ListFree(ctx, ownerID)
	}
	chain, err := uc.Nodes.Ancestors(ctx, ownerID, nodeID)
	if err != nil {
		return nil, err
	}
	// Nodes.Ancestors returns (nil, nil) — not an error — for an
	// unknown/foreign node id (rg-verified testutil.FakeNodeStore.Ancestors
	// and pgstore's mirrored owner-scoped WHERE). A valid node always has
	// len(chain) >= 1 (it contains itself), so an empty chain means
	// bogus/foreign: return nil rather than falling through to appending
	// ListFree, which would leak the caller's ENTIRE free library for a node
	// id that doesn't even belong to them (codex #2, KRITISCH).
	if len(chain) == 0 {
		return nil, nil
	}
	ids := make([]string, len(chain))
	for i, n := range chain {
		ids[i] = n.ID
	}
	nodeArts, err := uc.Artifacts.List(ctx, ownerID, ids...)
	if err != nil {
		return nil, err
	}
	free, err := uc.Artifacts.ListFree(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	return append(nodeArts, free...), nil // free appended last = root-lowest priority
}
