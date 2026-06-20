package daydetail

import (
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// dayRow is one session rendered as a list entry. Project name resolution is
// deferred to Task 6 when ListProjects joins the API interface; for now the
// Project field carries the raw ProjectID (or "" when unset).
type dayRow struct {
	ID      string
	Start   time.Time
	Stop    time.Time // zero when session is running
	Dur     time.Duration
	Running bool   // true when Stop was nil (session still active)
	Project string // ProjectID for now; resolved to name in Task 6
	Tag     string
}

// buildRows converts a slice of WorkSession into sorted dayRows. Sessions are
// sorted ascending by Start time. A nil-Stop (running) session is included with
// Dur=0 and Running=true, since a backfilled day normally has only completed
// sessions but the running entry should still appear if the user drilled into
// today.
func buildRows(sessions []domain.WorkSession, _ time.Time) []dayRow {
	rows := make([]dayRow, 0, len(sessions))
	for _, s := range sessions {
		r := dayRow{
			ID:  s.ID,
			Tag: s.Tag,
		}
		if s.ProjectID != nil {
			r.Project = *s.ProjectID
		}
		r.Start = s.Start
		if s.Stop != nil {
			r.Stop = *s.Stop
			r.Dur = r.Stop.Sub(r.Start)
		} else {
			r.Running = true
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Start.Before(rows[j].Start)
	})
	return rows
}
