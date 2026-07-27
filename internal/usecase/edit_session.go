package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// EditSessionInput carries the editable fields of an existing session.
// Tags is tri-state: nil = leave taggings untouched; &[] = clear; &[v...] = replace.
type EditSessionInput struct {
	NodeID *string
	Tags   *[]string
	Note   string
	Start  time.Time
	Stop   *time.Time
}

// EditSession overwrites a session's project/tags/note/times. Owner-scoped via
// the store. A set Stop must be strictly after Start.
type EditSession struct {
	Sessions ports.TransactionalSessionStore
	Nodes    ports.NodeStore
	Clock    ports.Clock
	Loc      *time.Location
	// Deprecated: session tags are written through Sessions.WithinTransaction.
	Tags ports.TagStore
}

func (uc EditSession) loc() *time.Location {
	if uc.Loc != nil {
		return uc.Loc
	}
	return time.Local
}

func (uc EditSession) Execute(ctx context.Context, ownerID, id string, in EditSessionInput) (domain.WorkSession, error) {
	if in.Stop != nil && !in.Stop.After(in.Start) {
		return domain.WorkSession{}, domain.ErrStopBeforeStart
	}
	// Existence check before overlap/validation: a non-existent or foreign-owned
	// session must return ErrSessionNotFound, not disclose conflicting data.
	current, err := uc.Sessions.Get(ctx, ownerID, id)
	if err != nil {
		return domain.WorkSession{}, err
	}
	now := time.Now()
	if uc.Clock != nil {
		now = uc.Clock.Now()
	}
	if in.Start.After(now) || (in.Stop != nil && in.Stop.After(now)) {
		return domain.WorkSession{}, domain.ErrFutureSession
	}
	if in.Stop != nil && !sameDayIn(in.Start, *in.Stop, uc.loc()) {
		return domain.WorkSession{}, fmt.Errorf("%w: start and stop must be on the same day", domain.ErrInvalidSession)
	}
	if err := requireBookable(ctx, uc.Nodes, ownerID, in.NodeID); err != nil {
		return domain.WorkSession{}, err
	}
	localStart := in.Start.In(uc.loc())
	dayStart := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, uc.loc())
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
	var updated domain.WorkSession
	err = uc.Sessions.WithinTransaction(ctx, func(tx ports.SessionWriter) error {
		updated, err = tx.Update(ctx, ownerID, id, in.NodeID, in.Note, in.Start, in.Stop)
		if err != nil {
			return err
		}
		if in.Tags != nil {
			updated.Tags, err = tx.SetTags(ctx, ownerID, id, *in.Tags)
		} else {
			updated.Tags = current.Tags
		}
		return err
	})
	if err != nil {
		return domain.WorkSession{}, err
	}
	return updated, nil
}
