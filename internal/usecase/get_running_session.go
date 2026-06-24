package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// GetRunningSession returns the owner's single running session (the timer that
// is currently open), if any — regardless of which day it started on. The
// WebUI "Heute" page uses this so a timer left running overnight stays visible
// and stoppable instead of silently blocking a new start. Owner-scoped.
type GetRunningSession struct {
	Sessions ports.SessionStore
}

func (uc GetRunningSession) Execute(ctx context.Context, ownerID string) (domain.WorkSession, bool, error) {
	return uc.Sessions.Running(ctx, ownerID)
}
