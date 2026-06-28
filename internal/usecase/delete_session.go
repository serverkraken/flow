package usecase

import (
	"context"
	"log/slog"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DeleteSession removes a session. Owner-scoped via the store.
type DeleteSession struct {
	Sessions ports.SessionStore
	Tags     ports.TagStore // optional; if non-nil, taggings are cleared on delete
}

func (uc DeleteSession) Execute(ctx context.Context, ownerID, id string) error {
	if err := uc.Sessions.Delete(ctx, ownerID, id); err != nil {
		return err
	}
	if uc.Tags != nil {
		if err := uc.Tags.ClearTaggable(ctx, ownerID, domain.TaggableWorkSession, id); err != nil {
			slog.WarnContext(ctx, "delete_session: clear taggings failed", "id", id, "err", err)
		}
	}
	return nil
}
