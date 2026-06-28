package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// StopSession ends a running session and books it to an engagement. Booking is
// mandatory; the engagement must already exist (clients inline-create via
// CreateNode first, then pass the new id here). A timer that has run across
// one or more local midnights is split into one session per calendar day, all
// booked to the same engagement, so each day's totals stay accurate.
type StopSession struct {
	Sessions ports.SessionStore
	Nodes	ports.NodeStore
	IDs      ports.IDGen
	Clock    ports.Clock
	Loc      *time.Location
	// Tags copies the original session's tags onto each split chunk via the
	// taggings junction after Create. Nil-safe: when unwired tags are silently dropped.
	Tags ports.TagStore
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
	n, err := uc.Nodes.Get(ctx, ownerID, *nodeID)
	if err != nil {
		return domain.WorkSession{}, err // ErrNodeNotFound bubbles to a 404
	}
	if n.Kind != domain.KindEngagement {
		return domain.WorkSession{}, fmt.Errorf("%w: worktime books to an engagement, got %s", domain.ErrInvalidNode, n.Kind)
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
		chunk.Tags, chunk.Note = cur.Tags, cur.Note
		if _, cerr := uc.Sessions.Create(ctx, chunk); cerr != nil {
			return first, cerr
		}
		if uc.Tags != nil {
			if _, serr := uc.Tags.SetTags(ctx, ownerID, domain.TaggableWorkSession, chunk.ID, cur.Tags); serr != nil {
				slog.WarnContext(ctx, "stop_session: failed to copy tags onto split chunk", "chunk", chunk.ID, "err", serr)
			}
		}
	}
	return first, nil
}
