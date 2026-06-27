package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// StartSession begins the user's single running timer. nodeID is optional at
// start; when set it must name an engagement (worktime books to engagements,
// D3). tag/note are optional annotations.
type StartSession struct {
	Sessions ports.SessionStore
	Nodes    ports.NodeStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc StartSession) Execute(ctx context.Context, ownerID string, nodeID *string, tag, note string) (domain.WorkSession, error) {
	if err := requireEngagement(ctx, uc.Nodes, ownerID, nodeID); err != nil {
		return domain.WorkSession{}, err
	}
	if _, running, err := uc.Sessions.Running(ctx, ownerID); err != nil {
		return domain.WorkSession{}, err
	} else if running {
		return domain.WorkSession{}, domain.ErrAlreadyRunning
	}
	s, err := domain.NewWorkSession(uc.IDs.NewID(), ownerID, nodeID, uc.Clock.Now())
	if err != nil {
		return domain.WorkSession{}, err
	}
	s.Tag, s.Note = tag, note
	return uc.Sessions.Create(ctx, s)
}
