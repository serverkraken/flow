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
	// Tags persists the session's tags into the taggings junction after the
	// session row is created. Nil-safe: when unwired, tags are dropped.
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
	created, err := uc.Sessions.Create(ctx, s)
	if err != nil {
		return domain.WorkSession{}, err
	}
	if uc.Tags != nil {
		t, terr := uc.Tags.SetTags(ctx, ownerID, domain.TaggableWorkSession, created.ID, tags)
		if terr != nil {
			return created, terr
		}
		created.Tags = slugsOf(t)
	}
	return created, nil
}
