package main

import (
	"time"

	"github.com/serverkraken/flow/internal/adapter/systemclock"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// minimalWorktimeReader is the slimmest WorktimeStatusReader the MCP
// server needs — flow_worktime_status only shows today's
// logged-time + target. We skip the full WorktimeReader (which pulls
// in ConfigReader + DayOffStore + Lock + LegacyActiveStore) because
// none of those signals are surfaced through MCP today. If a future
// tool needs richer worktime semantics, hoist the full reader.
type minimalWorktimeReader struct {
	sessions ports.SessionStore
	clock    systemclock.Clock
	// FixedTarget is the per-day target shown in flow_worktime_status
	// when no per-user config is loaded. 8h matches the default in
	// internal/adapter/iniconfig.
	fixedTarget time.Duration
}

func newMinimalWorktimeReader(sessions ports.SessionStore) usecase.WorktimeStatusReader {
	return &minimalWorktimeReader{
		sessions:    sessions,
		clock:       systemclock.New(),
		fixedTarget: 8 * time.Hour,
	}
}

func (r *minimalWorktimeReader) Today() (domain.Day, error) {
	now := r.clock.Now()
	day := domain.Day{Target: r.fixedTarget}
	sessions, err := r.sessions.LoadFiltered("", func(s domain.Session) bool {
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
