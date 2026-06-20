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
	dayStart := time.Date(in.Start.Year(), in.Start.Month(), in.Start.Day(), 0, 0, 0, 0, in.Start.Location())
	existing, err := uc.Sessions.ListRange(ctx, ownerID, dayStart.Add(-24*time.Hour), dayStart.Add(48*time.Hour))
	if err != nil {
		return domain.WorkSession{}, err
	}
	if domain.HasOverlap(existing, in.Start, in.Stop, id) {
		return domain.WorkSession{}, domain.ErrOverlap
	}
	return uc.Sessions.Update(ctx, ownerID, id, in.ProjectID, in.Tag, in.Note, in.Start, in.Stop)
}
