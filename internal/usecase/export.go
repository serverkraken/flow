package usecase

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// BuildExport aggregates a user's booked (stopped) sessions in [from,to] by
// engagement into a domain.ExportData, resolving engagement names + rates. The
// running session is excluded. engagementID "" means all engagements.
type BuildExport struct {
	Sessions ports.SessionStore
	Nodes    ports.NodeStore
	Clock    ports.Clock
	Loc      *time.Location
}

func (uc BuildExport) loc() *time.Location {
	if uc.Loc != nil {
		return uc.Loc
	}
	return time.Local
}

// Execute aggregates stopped sessions between from and to (inclusive day
// boundaries). engagementID filters to a single engagement when non-empty.
// Sessions store an engagement node_id post-migration, so grouping by NodeID
// already groups per engagement; the name/rate are resolved via NodeStore.
func (uc BuildExport) Execute(ctx context.Context, ownerID string, from, to time.Time, engagementID string) (domain.ExportData, error) {
	loc := uc.loc()
	// Inclusive day range: [from 00:00, to+1d 00:00)
	lo := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	toNorm := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc)
	hi := toNorm.AddDate(0, 0, 1)

	sessions, err := uc.Sessions.List(ctx, ownerID, lo)
	if err != nil {
		return domain.ExportData{}, err
	}
	projects, err := uc.Nodes.List(ctx, ownerID)
	if err != nil {
		return domain.ExportData{}, err
	}
	byID := make(map[string]domain.Node, len(projects))
	for _, p := range projects {
		byID[p.ID] = p
	}

	data := domain.ExportData{From: lo, To: toNorm}
	totals := map[string]*domain.NodeTotal{}

	for _, s := range sessions {
		// Exclude running sessions and sessions without a project booking.
		if s.Running() || s.NodeID == nil {
			continue
		}
		start := s.Start.In(loc)
		if start.Before(lo) || !start.Before(hi) {
			continue
		}
		if engagementID != "" && *s.NodeID != engagementID {
			continue
		}
		p := byID[*s.NodeID]
		name := p.Name
		if name == "" {
			name = "(unbekannt)"
		}
		stop := s.Stop.In(loc)
		el := stop.Sub(start)
		data.Sessions = append(data.Sessions, domain.ExportRow{
			Date:     start,
			Start:    start,
			Stop:     stop,
			Elapsed:  el,
			NodeName: name,
			Tag:      strings.Join(s.Tags, ","),
			Note:     s.Note,
		})
		t, ok := totals[*s.NodeID]
		if !ok {
			chain, aerr := uc.Nodes.Ancestors(ctx, ownerID, *s.NodeID)
			if aerr != nil {
				return domain.ExportData{}, aerr
			}
			t = &domain.NodeTotal{
				NodeID:   *s.NodeID,
				NodeName: name,
				Rate:     domain.ResolveRate(chain),
			}
			totals[*s.NodeID] = t
		}
		t.Total += el
		t.SessionCount++
	}

	for _, t := range totals {
		if t.Rate != nil {
			a := t.Rate.Mul(t.Total)
			t.Amount = &a
		}
		data.ByEngagement = append(data.ByEngagement, *t)
	}

	sort.Slice(data.ByEngagement, func(i, j int) bool {
		return data.ByEngagement[i].NodeName < data.ByEngagement[j].NodeName
	})
	sort.Slice(data.Sessions, func(i, j int) bool {
		return data.Sessions[i].Start.Before(data.Sessions[j].Start)
	})
	return data, nil
}
