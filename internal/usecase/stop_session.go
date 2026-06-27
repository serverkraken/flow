package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// StopSession ends a running session and books it to a project. Booking is
// mandatory; the project must already exist (clients inline-create via
// CreateNode first, then pass the new id here). A timer that has run across
// one or more local midnights is split into one session per calendar day, all
// booked to the same project, so each day's totals stay accurate.
type StopSession struct {
	Sessions ports.SessionStore
	Nodes	ports.NodeStore
	IDs      ports.IDGen
	Clock    ports.Clock
	Loc      *time.Location
}

func (uc StopSession) loc() *time.Location {
	if uc.Loc != nil {
		return uc.Loc
	}
	return time.Local
}

func (uc StopSession) Execute(ctx context.Context, ownerID, sessionID string, nodeID *string) (domain.WorkSession, error) {
	if nodeID == nil || *nodeID == "" {
		return domain.WorkSession{}, domain.ErrProjectRequired
	}
	if _, err := uc.Nodes.Get(ctx, ownerID, *nodeID); err != nil {
		return domain.WorkSession{}, err // ErrNodeNotFound bubbles to a 404
	}
	cur, err := uc.Sessions.Get(ctx, ownerID, sessionID)
	if err != nil {
		return domain.WorkSession{}, err
	}
	now := uc.Clock.Now()
	ranges := domain.SplitDaily(cur.Start, now, uc.loc())
	// Defensive: without an IDGen we cannot mint chunk ids, so stop the whole
	// span unsplit rather than panic (production always wires IDs; this guards
	// any future composition root / harness that does not).
	if uc.IDs == nil || len(ranges) == 1 {
		return uc.Sessions.Stop(ctx, ownerID, sessionID, nodeID, now)
	}
	// Stop the original session at the first day boundary, booking the project.
	first, err := uc.Sessions.Stop(ctx, ownerID, sessionID, nodeID, ranges[0].Stop)
	if err != nil {
		return domain.WorkSession{}, err
	}
	// One extra booked session per subsequent calendar day, same project/tag/note,
	// so a timer left running across midnight is attributed per day.
	for _, r := range ranges[1:] {
		chunk, nerr := domain.NewWorkSession(uc.IDs.NewID(), ownerID, nodeID, r.Start)
		if nerr != nil {
			return first, nerr
		}
		stop := r.Stop
		chunk.Stop = &stop
		chunk.Tag, chunk.Note = cur.Tag, cur.Note
		if _, cerr := uc.Sessions.Create(ctx, chunk); cerr != nil {
			return first, cerr
		}
	}
	return first, nil
}
