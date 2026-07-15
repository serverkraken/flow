package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// AddSession creates a complete (already-stopped) session for a past interval —
// "Nachbuchen". Unlike StartSession it takes explicit start/stop times. It
// enforces stop>start, no-future, same-day, and the no-overlap invariant.
// When nodeID is set it must name an engagement (worktime books to engagements,
// D3).
type AddSession struct {
	Sessions ports.TransactionalSessionStore
	Nodes    ports.NodeStore
	IDs      ports.IDGen
	Clock    ports.Clock
	Loc      *time.Location
	// Deprecated: session tags are written through Sessions.WithinTransaction.
	Tags ports.TagStore
}

func (uc AddSession) loc() *time.Location {
	if uc.Loc != nil {
		return uc.Loc
	}
	return time.Local
}

func (uc AddSession) Execute(ctx context.Context, ownerID string, nodeID *string, start, stop time.Time, tags []string, note string) (domain.WorkSession, error) {
	if err := requireBookable(ctx, uc.Nodes, ownerID, nodeID); err != nil {
		return domain.WorkSession{}, err
	}
	if !stop.After(start) {
		return domain.WorkSession{}, domain.ErrStopBeforeStart
	}
	now := uc.Clock.Now()
	if start.After(now) || stop.After(now) {
		return domain.WorkSession{}, domain.ErrFutureSession
	}
	if !sameDayIn(start, stop, uc.loc()) {
		return domain.WorkSession{}, fmt.Errorf("%w: start and stop must be on the same day", domain.ErrInvalidSession)
	}
	// Overlap check: pull the sessions around the candidate's day (±1 day to also
	// catch a cross-midnight neighbour) and apply the single-source rule.
	localStart := start.In(uc.loc())
	dayStart := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, uc.loc())
	existing, err := uc.Sessions.ListRange(ctx, ownerID, dayStart.Add(-24*time.Hour), dayStart.Add(48*time.Hour))
	if err != nil {
		return domain.WorkSession{}, err
	}
	// Also include any running session (Stop == nil): it may have started outside
	// the ListRange window but its interval is [start, +inf) and can still overlap
	// the candidate. ListRange filters on start_at only, so it would miss it.
	if run, ok, rerr := uc.Sessions.Running(ctx, ownerID); rerr != nil {
		return domain.WorkSession{}, rerr
	} else if ok {
		existing = append(existing, run)
	}
	if domain.HasOverlap(existing, start, &stop, "") {
		return domain.WorkSession{}, domain.ErrOverlap
	}
	s, err := domain.NewWorkSession(uc.IDs.NewID(), ownerID, nodeID, start)
	if err != nil {
		return domain.WorkSession{}, err
	}
	s.Stop = &stop
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

func sameDayIn(a, b time.Time, loc *time.Location) bool {
	ay, am, ad := a.In(loc).Date()
	by, bm, bd := b.In(loc).Date()
	return ay == by && am == bm && ad == bd
}
