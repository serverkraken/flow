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
	Sessions ports.TransactionalSessionStore
	Nodes    ports.NodeStore
	IDs      ports.IDGen
	Clock    ports.Clock
	// Deprecated: session tags are written through Sessions.WithinTransaction.
	Tags ports.TagStore
}

func (uc StartSession) Execute(ctx context.Context, ownerID string, nodeID *string, tags []string, note string) (domain.WorkSession, error) {
	if err := requireBookable(ctx, uc.Nodes, ownerID, nodeID); err != nil {
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
	s.Note = note
	var created domain.WorkSession
	err = uc.Sessions.WithinTransaction(ctx, func(tx ports.SessionWriter) error {
		created, err = tx.Create(ctx, s)
		if err != nil {
			return err
		}
		created.Tags, err = tx.SetTags(ctx, ownerID, created.ID, tags)
		return err
	})
	if err != nil {
		return domain.WorkSession{}, err
	}
	return created, nil
}
