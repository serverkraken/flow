package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DeleteArtifact removes one owner+node+slug-scoped artifact and emits
// artifact.deleted. No NodeStore dependency is needed — ArtifactStore.Delete
// is already owner+node+slug scoped, so a foreign owner or node simply yields
// ErrArtifactNotFound (Codex-Fund #1).
type DeleteArtifact struct {
	Artifacts ports.ArtifactStore
	Emitter   ports.Emitter
}

func (uc DeleteArtifact) Execute(ctx context.Context, ownerID, nodeID, slug string) error {
	if err := uc.Artifacts.Delete(ctx, ownerID, nodeID, slug); err != nil {
		return err
	}
	uc.Emitter.Emit(ctx, domain.Event{Type: domain.EventArtifactDeleted, UserID: ownerID, Data: map[string]any{"nodeId": nodeID, "slug": slug}})
	return nil
}
