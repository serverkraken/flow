package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

// DeleteSession removes a session. Owner-scoped via the store.
type DeleteSession struct {
	Sessions ports.TransactionalSessionStore
	Tags     ports.TagStore // Deprecated: tags are cleared inside the session transaction.
}

func (uc DeleteSession) Execute(ctx context.Context, ownerID, id string) error {
	return uc.Sessions.WithinTransaction(ctx, func(tx ports.SessionWriter) error {
		if _, err := tx.SetTags(ctx, ownerID, id, nil); err != nil {
			return err
		}
		return tx.Delete(ctx, ownerID, id)
	})
}
