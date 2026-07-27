package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListSessions returns the user's sessions started at or after since,
// newest first.
type ListSessions struct {
	Sessions ports.SessionStore
	Clock    ports.Clock
}

func (uc ListSessions) Execute(ctx context.Context, ownerID string, since time.Time) ([]domain.WorkSession, error) {
	return uc.Sessions.List(ctx, ownerID, since)
}
