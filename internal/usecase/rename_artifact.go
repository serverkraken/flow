package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// RenameArtifact changes an artifact's display name; slug, ref and bytes stay
// stable (so ![[slug]] references and cached URLs keep working). GetMeta
// confirms the artifact hangs off THIS node — not inherited via the ancestor
// chain — before renaming, and doubles as the owner+node+slug existence
// check (ErrArtifactNotFound covers both "missing" and "foreign").
type RenameArtifact struct {
	Nodes     ports.NodeStore
	Artifacts ports.ArtifactStore
	Emitter   ports.Emitter
}

func (uc RenameArtifact) Execute(ctx context.Context, ownerID, nodeID, slug, name string) error {
	if _, err := uc.Artifacts.GetMeta(ctx, ownerID, nodeID, slug); err != nil {
		return err
	}
	if err := uc.Artifacts.Rename(ctx, ownerID, nodeID, slug, name); err != nil {
		return err
	}
	uc.Emitter.Emit(ctx, domain.Event{Type: domain.EventArtifactUpdated, UserID: ownerID, Data: map[string]any{"nodeId": nodeID, "slug": slug}})
	return nil
}
