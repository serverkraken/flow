package usecase

import (
	"context"
	"log/slog"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DeleteNode removes a project. Owner-scoped via the store.
type DeleteNode struct {
	Nodes ports.NodeStore
	Tags  ports.TagStore
}

func (uc DeleteNode) Execute(ctx context.Context, ownerID, id string) error {
	if err := uc.Nodes.Delete(ctx, ownerID, id); err != nil {
		return err
	}
	// Best-effort: clear taggings after a successful node delete.
	if uc.Tags != nil {
		if err := uc.Tags.ClearTaggable(ctx, ownerID, domain.TaggableNode, id); err != nil {
			slog.WarnContext(ctx, "delete_node: clear taggings failed", "id", id, "err", err)
		}
	}
	return nil
}
