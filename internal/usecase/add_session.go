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
	Sessions ports.SessionStore
	Nodes    ports.NodeStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc AddSession) Execute(ctx context.Context, ownerID string, nodeID *string, start, stop time.Time, tag, note string) (domain.WorkSession, error) {
	if err := requireEngagement(ctx, uc.Nodes, ownerID, nodeID); err != nil {
		return domain.WorkSession{}, err
	}
	if !stop.After(start) {
		return domain.WorkSession{}, domain.ErrStopBeforeStart
	}
	now := uc.Clock.Now()
	if start.After(now) || stop.After(now) {
		return domain.WorkSession{}, domain.ErrFutureSession
	}
	if !sameLocalDay(start, stop) {
		return domain.WorkSession{}, fmt.Errorf("%w: start and stop must be on the same day", domain.ErrInvalidSession)
	}
	// Overlap check: pull the sessions around the candidate's day (±1 day to also
	// catch a cross-midnight neighbour) and apply the single-source rule.
	dayStart := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
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
	s.Tag, s.Note = tag, note
	return uc.Sessions.Create(ctx, s)
}

// sameLocalDay reports whether a and b fall on the same calendar day in their
// own locations.
func sameLocalDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
