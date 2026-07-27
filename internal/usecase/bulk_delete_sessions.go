package usecase

import (
	"context"
	"errors"

	"github.com/serverkraken/flow/internal/ports"
)

// BulkDeleteSessions deletes many sessions at once (import cleanup). Owner-scoped;
// missing/foreign ids are skipped. Returns the count actually deleted.
type BulkDeleteSessions struct {
	Sessions ports.TransactionalSessionStore
	Tags     ports.TagStore // Deprecated: tags are cleared inside each session transaction.
}

func (uc BulkDeleteSessions) Execute(ctx context.Context, ownerID string, ids []string) (int, error) {
	var err error
	ids, err = normalizedSessionIDs(ids)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, id := range ids {
		err = uc.Sessions.WithinTransaction(ctx, func(tx ports.SessionWriter) error {
			if _, err := tx.SetTags(ctx, ownerID, id, nil); err != nil {
				return err
			}
			return tx.Delete(ctx, ownerID, id)
		})
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
