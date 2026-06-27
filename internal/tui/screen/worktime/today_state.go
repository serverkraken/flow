package worktime

import (
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

type completedSession struct {
	ID        string
	Start     time.Time
	Stop      time.Time
	Elapsed   time.Duration
	Tag       string
	Note      string
	Project   string // resolved project name ("" if none or unknown)
	GapBefore time.Duration
}

type todayState struct {
	Completed []completedSession
	Running   bool
	Active    *time.Time
	ActiveID  string
	Logged    time.Duration
	Target    time.Duration
}

func sameLocalDay(a, b time.Time) bool {
	b = b.In(a.Location())
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func reconstruct(today apiclient.Today, sessions []domain.WorkSession, projects []domain.Node, now time.Time) todayState {
	st := todayState{Target: time.Duration(today.TargetMin) * time.Minute, Running: today.Running}

	nameByID := make(map[string]string, len(projects))
	for _, p := range projects {
		nameByID[p.ID] = p.Name
	}

	todays := make([]domain.WorkSession, 0, len(sessions))
	for _, s := range sessions {
		if sameLocalDay(now, s.Start) {
			todays = append(todays, s)
		}
	}
	sort.Slice(todays, func(i, j int) bool { return todays[i].Start.Before(todays[j].Start) })

	var prevStop time.Time
	for _, s := range todays {
		if s.Running() {
			start := s.Start
			st.Active = &start
			st.ActiveID = s.ID
			st.Running = true
			continue
		}
		gap := time.Duration(0)
		if !prevStop.IsZero() {
			if g := s.Start.Sub(prevStop); g > 0 {
				gap = g
			}
		}
		el := s.Stop.Sub(s.Start)
		project := ""
		if s.NodeID != nil {
			project = nameByID[*s.NodeID]
		}
		st.Completed = append(st.Completed, completedSession{
			ID: s.ID, Start: s.Start, Stop: *s.Stop, Elapsed: el,
			Tag: s.Tag, Note: s.Note, Project: project, GapBefore: gap,
		})
		st.Logged += el
		prevStop = *s.Stop
	}
	return st
}

func (st todayState) Total(now time.Time) time.Duration {
	t := st.Logged
	if st.Running && st.Active != nil {
		if d := now.Sub(*st.Active); d > 0 {
			t += d
		}
	}
	return t
}

func (st todayState) ETA() (time.Time, bool) {
	if !st.Running || st.Active == nil || st.Target <= 0 {
		return time.Time{}, false
	}
	return st.Active.Add(st.Target - st.Logged), true
}
