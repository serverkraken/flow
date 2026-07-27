package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// GetArtifact fetches one owner+node+slug-scoped artifact including its
// bytes, for the Serve route. Read-only; no event, no NodeStore dependency
// (ArtifactStore.Get is already owner+node+slug scoped — Codex-Fund #1).
type GetArtifact struct {
	Artifacts ports.ArtifactStore
}

func (uc GetArtifact) Execute(ctx context.Context, ownerID, nodeID, slug string) (domain.Artifact, error) {
	return uc.Artifacts.Get(ctx, ownerID, nodeID, slug)
}
