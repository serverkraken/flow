package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListSessionsRange returns the owner's sessions with since <= Start < until,
// newest first. Used by past-day views.
type ListSessionsRange struct {
	Sessions ports.SessionStore
}

func (uc ListSessionsRange) Execute(ctx context.Context, ownerID string, since, until time.Time) ([]domain.WorkSession, error) {
	return uc.Sessions.ListRange(ctx, ownerID, since, until)
}
