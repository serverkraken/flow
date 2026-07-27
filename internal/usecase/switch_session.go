package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SwitchSession atomically stops the current timer (including midnight
// splitting) and starts a new timer on targetNodeID. If no timer is running it
// behaves like StartSession. No observer can see only one half committed.
type SwitchSession struct {
	Sessions ports.TransactionalSessionStore
	Nodes    ports.NodeStore
	IDs      ports.IDGen
	Clock    ports.Clock
	Loc      *time.Location
}

func (uc SwitchSession) loc() *time.Location {
	if uc.Loc != nil {
		return uc.Loc
	}
	return time.Local
}

func (uc SwitchSession) Execute(ctx context.Context, ownerID string, targetNodeID *string) (domain.WorkSession, bool, domain.WorkSession, error) {
	if targetNodeID == nil || *targetNodeID == "" {
		return domain.WorkSession{}, false, domain.WorkSession{}, domain.ErrProjectRequired
	}
	if err := requireBookable(ctx, uc.Nodes, ownerID, targetNodeID); err != nil {
		return domain.WorkSession{}, false, domain.WorkSession{}, err
	}

	now := uc.Clock.Now()
	current, running, err := uc.Sessions.Running(ctx, ownerID)
	if err != nil {
		return domain.WorkSession{}, false, domain.WorkSession{}, err
	}
	next, err := domain.NewWorkSession(uc.IDs.NewID(), ownerID, targetNodeID, now)
	if err != nil {
		return domain.WorkSession{}, false, domain.WorkSession{}, err
	}

	var plan stopPlan
	if running {
		stopNode := current.NodeID
		if stopNode == nil {
			stopNode = targetNodeID
		}
		if err := requireBookable(ctx, uc.Nodes, ownerID, stopNode); err != nil {
			return domain.WorkSession{}, false, domain.WorkSession{}, err
		}
		plan, err = buildStopPlan(current, stopNode, now, uc.loc(), uc.IDs)
		if err != nil {
			return domain.WorkSession{}, false, domain.WorkSession{}, err
		}
	}

	var stopped, started domain.WorkSession
	err = uc.Sessions.WithinTransaction(ctx, func(tx ports.SessionWriter) error {
		if running {
			stopped, err = persistStopPlan(ctx, tx, plan)
			if err != nil {
				return err
			}
		}
		started, err = tx.Create(ctx, next)
		if err != nil {
			return err
		}
		started.Tags, err = tx.SetTags(ctx, ownerID, started.ID, nil)
		return err
	})
	if err != nil {
		return domain.WorkSession{}, false, domain.WorkSession{}, err
	}
	return stopped, running, started, nil
}
