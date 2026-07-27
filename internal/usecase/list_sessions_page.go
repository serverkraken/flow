package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListSessionsPage returns one page of the owner's sessions (newest-first) plus
// the total count, for the WebUI "Alle Sitzungen" list. Owner-scoped via store.
type ListSessionsPage struct {
	Sessions ports.SessionStore
}

func (uc ListSessionsPage) Execute(ctx context.Context, ownerID string, limit, offset int) ([]domain.WorkSession, int, error) {
	return uc.Sessions.ListPage(ctx, ownerID, limit, offset)
}
