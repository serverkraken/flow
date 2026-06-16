package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// EditSessionInput carries the editable fields of an existing session.
type EditSessionInput struct {
	ProjectID *string
	Tag       string
	Note      string
	Start     time.Time
	Stop      *time.Time
}

// EditSession overwrites a session's project/tag/note/times. Owner-scoped via
// the store. A set Stop must be strictly after Start.
type EditSession struct {
	Sessions ports.SessionStore
}

func (uc EditSession) Execute(ctx context.Context, ownerID, id string, in EditSessionInput) (domain.WorkSession, error) {
	if in.Stop != nil && !in.Stop.After(in.Start) {
		return domain.WorkSession{}, domain.ErrStopBeforeStart
	}
	return uc.Sessions.Update(ctx, ownerID, id, in.ProjectID, in.Tag, in.Note, in.Start, in.Stop)
}
