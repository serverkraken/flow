package usecase

import (
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// WorktimeReader is the read-only entry point to the worktime data layer.
// All four read shapes (today, week, history, range) come through here.
type WorktimeReader struct {
	Sessions ports.SessionStore
	State    ports.LegacyActiveStore
	Targets  *TargetResolver
	Clock    ports.Clock

	// ShowWeekend, when true, always renders Sat/Sun in week views even
	// when they have no sessions and aren't today.
	ShowWeekend bool
}

// Today returns the day record for "today" — sessions logged so far,
// active session marker (if any), pause marker (if no active session),
// and the resolved daily target.
func (r *WorktimeReader) Today() (domain.Day, error) {
	now := r.Clock.Now()
	day := domain.Day{Target: r.Targets.For(now)}

	active, err := r.State.GetActive()
	if err != nil {
		return day, err
	}
	if active != nil {
		day.Active = active
	} else {
		pause, err := r.State.GetPause()
		if err != nil {
			return day, err
		}
		if pause != nil {
			day.PausedAt = pause
		}
	}

	sessions, err := r.Sessions.LoadFiltered("", func(s domain.Session) bool {
		return domain.SameDay(s.Date, now)
	})
	if err != nil {
		return day, err
	}
	day.Sessions = sessions
	for _, s := range sessions {
		day.Logged += s.Elapsed
	}
	return day, nil
}

// Week returns Mon–Sun of the current week. Saturday/Sunday rows are
// dropped when they have no sessions and aren't today, unless ShowWeekend
// is set.
func (r *WorktimeReader) Week() ([]domain.WeekDay, error) {
	now := r.Clock.Now()
	active, err := r.State.GetActive()
	if err != nil {
		return nil, err
	}

	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	monday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, -(wd - 1))
	sunday := monday.AddDate(0, 0, 6)

	allSessions, err := r.Sessions.LoadFiltered("", func(s domain.Session) bool {
		return !s.Date.Before(monday) && !s.Date.After(sunday)
	})
	if err != nil {
		return nil, err
	}

	byDay := make(map[string]time.Duration)
	for _, s := range allSessions {
		byDay[s.Date.Format("2006-01-02")] += s.Elapsed
	}

	var week []domain.WeekDay
	for i := 0; i < 7; i++ {
		day := monday.AddDate(0, 0, i)
		isToday := domain.SameDay(day, now)
		logged := byDay[day.Format("2006-01-02")]
		isWeekend := i >= 5

		if isWeekend && logged == 0 && !isToday && !r.ShowWeekend {
			continue
		}

		var dayActive *time.Time
		if isToday {
			dayActive = active
		}
		week = append(week, domain.WeekDay{
			Date:    day,
			Logged:  logged,
			Active:  dayActive,
			Target:  r.Targets.For(day),
			IsToday: isToday,
		})
	}
	return week, nil
}

// History returns every day with at least one session, newest first.
func (r *WorktimeReader) History() ([]domain.DayRecord, error) {
	sessions, err := r.Sessions.Load("")
	if err != nil {
		return nil, err
	}

	byDate := make(map[string]*domain.DayRecord)
	var order []string
	for _, s := range sessions {
		key := s.Date.Format("2006-01-02")
		if _, ok := byDate[key]; !ok {
			byDate[key] = &domain.DayRecord{Date: s.Date, Target: r.Targets.For(s.Date)}
			order = append(order, key)
		}
		rec := byDate[key]
		rec.Sessions = append(rec.Sessions, s)
		rec.Total += s.Elapsed
	}

	sort.Sort(sort.Reverse(sort.StringSlice(order)))
	out := make([]domain.DayRecord, len(order))
	for i, key := range order {
		out[i] = *byDate[key]
	}
	return out, nil
}

// Range returns sessions whose Date falls inside r. Empty range means
// "all sessions".
func (r *WorktimeReader) Range(rng domain.Range) ([]domain.Session, error) {
	if rng.From.IsZero() && rng.To.IsZero() {
		return r.Sessions.Load("")
	}
	return r.Sessions.LoadFiltered("", func(s domain.Session) bool {
		return rng.ContainsDate(s.Date)
	})
}

// SessionsOverlap reports whether the candidate span [start, stop)
// intersects any session on `date`, except the one at excludeIdx (use -1
// to consider all). Returns the conflicting session as a pointer for the
// UI to surface a precise hint.
func (r *WorktimeReader) SessionsOverlap(date, start, stop time.Time, excludeIdx int) (bool, *domain.Session, error) {
	// LoadFiltered limits I/O to the rows of the target day. The previous
	// LoadAll read every row in sessions.tsv even though all but ~3 were
	// going to be skipped — on a 2-year history (~1500 rows) every
	// AddManual paid that scan. The filter predicate runs in the adapter
	// where it can short-circuit during line parsing.
	dateStr := date.Format("2006-01-02")
	dayRows, err := r.Sessions.LoadFiltered("", func(s domain.Session) bool {
		return s.Date.Format("2006-01-02") == dateStr
	})
	if err != nil {
		return false, nil, err
	}
	for i, s := range dayRows {
		if i == excludeIdx {
			continue
		}
		if start.Before(s.Stop) && s.Start.Before(stop) {
			return true, &dayRows[i], nil
		}
	}
	return false, nil, nil
}
