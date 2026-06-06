package handlers

import (
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/webui/format"
	projectstmpl "github.com/serverkraken/flow/internal/webui/templates/projects"
	projectspartials "github.com/serverkraken/flow/internal/webui/templates/projects/partials"
)

// buildProjectsIndexVM resolves the data for the /projects index. It
// pulls all projects (active + archived) plus this week's sessions to
// aggregate the per-row Diese-Woche / Sessions counters, then folds in
// the user's single active session so the row carrying it gets the ▶
// glyph.
//
// The active-since-7d count for the page eyebrow is computed off a
// 7-day lookback into sessions, distinct project ids.
func buildProjectsIndexVM(d ProjectsDeps, userID string, tab projectsSubTab, now time.Time) (projectstmpl.IndexVM, error) {
	vm := projectstmpl.IndexVM{
		ActiveTab: string(tab),
	}

	allProjects, err := d.Projects.ListAll(userID)
	if err != nil {
		return vm, fmt.Errorf("ListAll projects: %w", err)
	}

	// 7-day lookback for the page eyebrow and per-row "Diese Woche"
	// columns. Inclusive both ends — ListByUserDateRange compares the
	// stored YYYY-MM-DD date column.
	weekStart := format.MondayOf(now)
	weekEnd := weekStart.AddDate(0, 0, 6)
	since := dayOf(now).AddDate(0, 0, -6)
	sevenDay, err := d.Sessions.ListByUserDateRange(userID, since, dayOf(now))
	if err != nil {
		return vm, fmt.Errorf("sessions 7d: %w", err)
	}
	weekSessions, err := d.Sessions.ListByUserDateRange(userID, weekStart, weekEnd)
	if err != nil {
		return vm, fmt.Errorf("sessions week: %w", err)
	}

	// At most one active session per user (server invariant). Look it
	// up so the row carrying it can render the ▶ glyph.
	activeProjectID := ""
	if d.Active != nil {
		rows, err := d.Active.ListByUser(userID)
		if err != nil {
			return vm, fmt.Errorf("active list: %w", err)
		}
		if len(rows) > 0 {
			activeProjectID = rows[0].ProjectID
		}
	}

	// Aggregate week stats per project once, then index by project ID.
	weekByProject := aggregateWeekByProject(weekSessions)
	lastByProject := lastActivityByProject(sevenDay)

	vm.Rows = make([]projectspartials.ProjectRowVM, 0, len(allProjects))
	for _, p := range allProjects {
		row := projectRow(p, activeProjectID, lastByProject, weekByProject, now)
		vm.Rows = append(vm.Rows, row)
	}

	// Page eyebrow: total projects + active in last 7 days. "Aktiv"
	// means: had at least one session in the 7-day lookback OR is the
	// currently-running active session.
	activeSet := map[string]struct{}{}
	for _, s := range sevenDay {
		activeSet[s.ProjectID] = struct{}{}
	}
	if activeProjectID != "" {
		activeSet[activeProjectID] = struct{}{}
	}
	vm.TotalLabel = projectstmpl.FormatTotalsLabel(len(allProjects), len(activeSet))
	vm.HasProjects = len(allProjects) > 0
	return vm, nil
}

// dayOf returns 00:00 of t in t's location.
func dayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// aggregateWeekByProject sums elapsed + session count per project for
// the given session slice. The active session's tail isn't included —
// the row picks it up via the ▶ glyph and "jetzt" relative-time.
func aggregateWeekByProject(sessions []domain.Session) map[string]projectWeekStats {
	out := map[string]projectWeekStats{}
	for _, s := range sessions {
		ws := out[s.ProjectID]
		ws.Total += s.Elapsed
		ws.Count++
		out[s.ProjectID] = ws
	}
	return out
}

// lastActivityByProject returns the most-recent session Stop per
// project across the 7-day lookback. Used for the "Zuletzt" column.
func lastActivityByProject(sessions []domain.Session) map[string]time.Time {
	out := map[string]time.Time{}
	for _, s := range sessions {
		when := s.Stop
		if when.IsZero() {
			when = s.Start
		}
		if prev, ok := out[s.ProjectID]; !ok || when.After(prev) {
			out[s.ProjectID] = when
		}
	}
	return out
}

// projectWeekStats holds the per-row aggregates for the current week.
type projectWeekStats struct {
	Total time.Duration
	Count int
}

// projectRow assembles one /projects list row from a project + the
// pre-aggregated maps. Pure formatting; no I/O.
func projectRow(
	p domain.Project,
	activeProjectID string,
	lastByProject map[string]time.Time,
	weekByProject map[string]projectWeekStats,
	now time.Time,
) projectspartials.ProjectRowVM {
	isActive := activeProjectID == p.ID
	isArchived := p.ArchivedAt != nil
	weekStats := weekByProject[p.ID]

	row := projectspartials.ProjectRowVM{
		ID:              p.ID,
		Name:            p.Name,
		Slug:            p.Slug,
		Archived:        isArchived,
		HasActive:       isActive,
		Version:         p.Version,
		WeekDuration:    "—",
		WeekCount:       "0",
		LastLabel:       "—",
		WeekDurationDim: true,
		LastDim:         true,
	}

	if weekStats.Count > 0 {
		row.WeekDuration = format.HHMM(weekStats.Total)
		row.WeekDurationDim = false
		row.WeekCount = fmt.Sprintf("%d", weekStats.Count)
	}

	if isActive {
		row.LastLabel = "jetzt"
		row.LastDim = false
		row.LiveAccent = true
	} else if last, ok := lastByProject[p.ID]; ok {
		row.LastLabel = format.HumanRelativeTime(last, now)
		row.LastDim = false
	} else if isArchived {
		row.LastLabel = "archiviert"
	}

	return row
}
