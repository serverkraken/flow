package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// BuildExport aggregates a user's booked (stopped) sessions in [from,to] by
// project into a domain.ExportData, resolving project names + rates. The
// running session is excluded. nodeID "" means all projects.
type BuildExport struct {
	Sessions ports.SessionStore
	Nodes	ports.NodeStore
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
// boundaries). nodeID filters to a single project when non-empty.
func (uc BuildExport) Execute(ctx context.Context, ownerID string, from, to time.Time, nodeID string) (domain.ExportData, error) {
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
		if nodeID != "" && *s.NodeID != nodeID {
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
			Date:        start,
			Start:       start,
			Stop:        stop,
			Elapsed:     el,
			NodeName: name,
			Tag:         s.Tag,
			Note:        s.Note,
		})
		t, ok := totals[*s.NodeID]
		if !ok {
			t = &domain.NodeTotal{
				NodeID:   *s.NodeID,
				NodeName: name,
				Rate:        p.Rate,
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
