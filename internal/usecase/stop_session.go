package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// StopSession ends a running session and books it to a project. Booking is
// mandatory; the project must already exist (clients inline-create via
// CreateProject first, then pass the new id here).
type StopSession struct {
	Sessions ports.SessionStore
	Projects ports.ProjectStore
	Clock    ports.Clock
}

func (uc StopSession) Execute(ctx context.Context, ownerID, sessionID string, projectID *string) (domain.WorkSession, error) {
	if projectID == nil || *projectID == "" {
		return domain.WorkSession{}, domain.ErrProjectRequired
	}
	if _, err := uc.Projects.Get(ctx, ownerID, *projectID); err != nil {
		return domain.WorkSession{}, err // ErrProjectNotFound bubbles to a 404
	}
	return uc.Sessions.Stop(ctx, ownerID, sessionID, projectID, uc.Clock.Now())
}
