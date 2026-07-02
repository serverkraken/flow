package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// NodeStats rolls a node's worktime up over its subtree (own sessions + all
// descendants'), bucketed into Total / current-ISO-week / current-month, and
// splits each bucket into Work / Privat by each session's node's effective
// CountsTowardTarget flag (domain.ResolveCountsTowardTarget).
func (c StatsComputer) NodeStats(ctx context.Context, ownerID, nodeID string) (domain.NodeRollup, error) {
	sub, err := c.Nodes.Subtree(ctx, ownerID, nodeID)
	if err != nil {
		return domain.NodeRollup{}, err
	}
	if len(sub) == 0 {
		return domain.NodeRollup{}, ports.ErrNodeNotFound
	}
	// Effective Work/Privat flag per subtree node. Base = resolved from the
	// cockpit node's ancestors (covers "all-nil inherits from above the subtree").
	anc, _ := c.Nodes.Ancestors(ctx, ownerID, nodeID)
	base := domain.ResolveCountsTowardTarget(anc)
	eff := make(map[string]bool, len(sub))
	for _, n := range sub { // Subtree is depth-ordered root->leaf: parents precede children
		switch {
		case n.CountsTowardTarget != nil:
			eff[n.ID] = *n.CountsTowardTarget
		case n.ParentID != nil:
			if pv, ok := eff[*n.ParentID]; ok {
				eff[n.ID] = pv
			} else {
				eff[n.ID] = base
			}
		default:
			eff[n.ID] = base
		}
	}
	ids := make(map[string]bool, len(sub))
	for _, n := range sub {
		ids[n.ID] = true
	}
	sessions, err := c.Sessions.List(ctx, ownerID, time.Time{})
	if err != nil {
		return domain.NodeRollup{}, err
	}
	loc := c.Loc
	if loc == nil {
		loc = time.Local
	}
	now := c.Clock.Now().In(loc)
	weekStart := isoMondayLocal(now)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	var r domain.NodeRollup
	for _, s := range sessions {
		if s.NodeID == nil || !ids[*s.NodeID] {
			continue
		}
		el := s.Elapsed(now)
		if el < 0 {
			el = 0
		}
		work := eff[*s.NodeID]
		st := s.Start.In(loc)
		inWeek := !st.Before(weekStart)
		inMonth := !st.Before(monthStart)
		r.Total += el
		if inWeek {
			r.Week += el
		}
		if inMonth {
			r.Month += el
		}
		if work {
			r.WorkTotal += el
			if inWeek {
				r.WorkWeek += el
			}
			if inMonth {
				r.WorkMonth += el
			}
		}
	}
	return r, nil
}
