package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// NodeStats rolls a node's worktime up over its subtree (own sessions + all
// descendants'), bucketed into Total / current-ISO-week / current-month.
func (c StatsComputer) NodeStats(ctx context.Context, ownerID, nodeID string) (domain.NodeRollup, error) {
	sub, err := c.Nodes.Subtree(ctx, ownerID, nodeID)
	if err != nil {
		return domain.NodeRollup{}, err
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
		r.Total += el
		st := s.Start.In(loc)
		if !st.Before(weekStart) {
			r.Week += el
		}
		if !st.Before(monthStart) {
			r.Month += el
		}
	}
	return r, nil
}
