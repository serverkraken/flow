package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

// DeleteSession removes a session. Owner-scoped via the store.
type DeleteSession struct {
	Sessions ports.SessionStore
}

func (uc DeleteSession) Execute(ctx context.Context, ownerID, id string) error {
	return uc.Sessions.Delete(ctx, ownerID, id)
}
