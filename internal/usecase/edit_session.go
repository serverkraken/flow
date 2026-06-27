package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// EditSessionInput carries the editable fields of an existing session.
type EditSessionInput struct {
	NodeID *string
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
	// Existence check before overlap: a non-existent or foreign-owned session
	// must return ErrSessionNotFound (→ 404), not ErrOverlap (→ 409).
	if _, err := uc.Sessions.Get(ctx, ownerID, id); err != nil {
		return domain.WorkSession{}, err
	}
	dayStart := time.Date(in.Start.Year(), in.Start.Month(), in.Start.Day(), 0, 0, 0, 0, in.Start.Location())
	existing, err := uc.Sessions.ListRange(ctx, ownerID, dayStart.Add(-24*time.Hour), dayStart.Add(48*time.Hour))
	if err != nil {
		return domain.WorkSession{}, err
	}
	// Also include any running session (Stop == nil): it may have started outside
	// the ListRange window but its interval is [start, +inf) and can still overlap
	// the candidate. The existing excludeID safely prevents self-collision because
	// a running session being edited carries a different id than id.
	if run, ok, rerr := uc.Sessions.Running(ctx, ownerID); rerr != nil {
		return domain.WorkSession{}, rerr
	} else if ok {
		existing = append(existing, run)
	}
	if domain.HasOverlap(existing, in.Start, in.Stop, id) {
		return domain.WorkSession{}, domain.ErrOverlap
	}
	return uc.Sessions.Update(ctx, ownerID, id, in.NodeID, in.Tag, in.Note, in.Start, in.Stop)
}
