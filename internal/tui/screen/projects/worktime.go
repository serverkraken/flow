package projects

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

type worktimeAgg struct {
	Total, Week, Month time.Duration
	Earnings           string // "" when no rate
}

// aggregate sums settled sessions for project id. Mirrors the WebUI cockpit:
// running sessions are excluded (settled-time summary); earnings = rate × total.
func aggregate(p domain.Project, sessions []domain.WorkSession, now time.Time) worktimeAgg {
	weekStart := startOfWeek(now)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	var a worktimeAgg
	for _, s := range sessions {
		if s.ProjectID == nil || *s.ProjectID != p.ID || s.Running() {
			continue
		}
		d := s.Elapsed(now)
		a.Total += d
		if !s.Start.Before(weekStart) {
			a.Week += d
		}
		if !s.Start.Before(monthStart) {
			a.Month += d
		}
	}
	if p.Rate != nil {
		a.Earnings = p.Rate.Mul(a.Total).String()
	}
	return a
}

// startOfWeek returns the ISO Monday 00:00:00 of the week containing t (local).
func startOfWeek(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	// ISO week: Monday start. Weekday() Sunday==0 → shift to Monday==0 by (wd+6)%7.
	off := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -off)
}
