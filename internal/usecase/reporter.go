package usecase

import (
	"io"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Reporter writes serialised reports of the worktime data: a Stand-up
// Markdown brief, a CSV/JSON dump of sessions, or an iCal export of
// day-offs. All four orchestrate existing read services + a domain
// formatter.
type Reporter struct {
	Reader  *WorktimeReader
	DayOffs ports.DayOffStore
	Targets *TargetResolver
	Stats   *StatsComputer
	Clock   ports.Clock
}

// WriteBrief writes a Stand-up-ready Markdown summary of the range
// containing ref to w. Loads history + dayoffs, computes stats, then
// hands a BriefInputs snapshot to domain.WriteBrief.
func (r *Reporter) WriteBrief(w io.Writer, ref time.Time, scope domain.ReportRange) error {
	from, to, title := domain.BriefBounds(ref, scope)
	hist, err := r.Reader.History()
	if err != nil {
		return err
	}
	records := domain.FilterRecords(hist, from, to)
	return domain.WriteBrief(w, domain.BriefInputs{
		Title:   title,
		Records: records,
		Stats:   r.Stats.Aggregate(records),
		Planned: domain.PlannedTarget(from, to, r.Targets.IsWorkday, r.Targets.For),
		// to is exclusive; ListDayOffs takes inclusive bounds.
		DayOffs: r.DayOffs.List(from, to.AddDate(0, 0, -1)),
	})
}

// WriteCSV writes a CSV dump of sessions in rng to w.
func (r *Reporter) WriteCSV(w io.Writer, rng domain.Range) error {
	sessions, err := r.Reader.Range(rng)
	if err != nil {
		return err
	}
	return domain.WriteCSV(w, sessions)
}

// WriteJSON writes a JSON array of sessions in rng to w.
func (r *Reporter) WriteJSON(w io.Writer, rng domain.Range) error {
	sessions, err := r.Reader.Range(rng)
	if err != nil {
		return err
	}
	return domain.WriteJSON(w, sessions)
}

// WriteICS writes an RFC 5545 calendar of day-offs in [from, to] to w.
func (r *Reporter) WriteICS(w io.Writer, from, to time.Time) error {
	return domain.WriteICS(w, r.DayOffs.List(from, to), r.Clock.Now())
}
