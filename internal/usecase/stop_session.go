package usecase

import (
	"context"
	"fmt"
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
	Sessions ports.TransactionalSessionStore
	Nodes    ports.NodeStore
	IDs      ports.IDGen
	Clock    ports.Clock
	Loc      *time.Location
	// Deprecated: split tags are written through Sessions.WithinTransaction.
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
	if !domain.IsBookable(n.Kind) {
		return domain.WorkSession{}, fmt.Errorf("%w: worktime books to a bookable node, got %s", domain.ErrInvalidNode, n.Kind)
	}
	cur, err := uc.Sessions.Get(ctx, ownerID, sessionID)
	if err != nil {
		return domain.WorkSession{}, err
	}
	now := uc.Clock.Now()
	plan, err := buildStopPlan(cur, nodeID, now, uc.loc(), uc.IDs)
	if err != nil {
		return domain.WorkSession{}, err
	}
	var first domain.WorkSession
	err = uc.Sessions.WithinTransaction(ctx, func(tx ports.SessionWriter) error {
		first, err = persistStopPlan(ctx, tx, plan)
		return err
	})
	if err != nil {
		return domain.WorkSession{}, err
	}
	return first, nil
}

type stopPlan struct {
	ownerID  string
	session  domain.WorkSession
	nodeID   *string
	firstEnd time.Time
	chunks   []domain.WorkSession
}

func buildStopPlan(cur domain.WorkSession, nodeID *string, now time.Time, loc *time.Location, ids ports.IDGen) (stopPlan, error) {
	ranges := domain.SplitDaily(cur.Start, now, loc)
	plan := stopPlan{ownerID: cur.OwnerID, session: cur, nodeID: nodeID, firstEnd: now}
	if ids == nil || len(ranges) <= 1 {
		return plan, nil
	}
	plan.firstEnd = ranges[0].Stop
	for _, r := range ranges[1:] {
		chunk, err := domain.NewWorkSession(ids.NewID(), cur.OwnerID, nodeID, r.Start)
		if err != nil {
			return stopPlan{}, err
		}
		stop := r.Stop
		chunk.Stop = &stop
		chunk.Tags = append([]string(nil), cur.Tags...)
		chunk.Note = cur.Note
		plan.chunks = append(plan.chunks, chunk)
	}
	return plan, nil
}

func persistStopPlan(ctx context.Context, tx ports.SessionWriter, plan stopPlan) (domain.WorkSession, error) {
	first, err := tx.Stop(ctx, plan.ownerID, plan.session.ID, plan.nodeID, plan.firstEnd)
	if err != nil {
		return domain.WorkSession{}, err
	}
	first.Tags = append([]string(nil), plan.session.Tags...)
	for _, chunk := range plan.chunks {
		created, err := tx.Create(ctx, chunk)
		if err != nil {
			return domain.WorkSession{}, err
		}
		if _, err := tx.SetTags(ctx, plan.ownerID, created.ID, chunk.Tags); err != nil {
			return domain.WorkSession{}, err
		}
	}
	return first, nil
}
