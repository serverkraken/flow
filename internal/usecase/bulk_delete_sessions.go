package usecase

import (
	"context"
	"errors"

	"github.com/serverkraken/flow/internal/ports"
)

// BulkDeleteSessions deletes many sessions at once (import cleanup). Owner-scoped;
// missing/foreign ids are skipped. Returns the count actually deleted.
type BulkDeleteSessions struct {
	Sessions ports.SessionStore
}

func (uc BulkDeleteSessions) Execute(ctx context.Context, ownerID string, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, ErrNoSessions
	}
	deleted := 0
	for _, id := range ids {
		err := uc.Sessions.Delete(ctx, ownerID, id)
		if errors.Is(err, ports.ErrSessionNotFound) {
			continue
		}
		if err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}
