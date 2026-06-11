package domain

import "time"

// ActiveSession is the in-progress worktime tracker for one (User, Project)
// pair. Multiple may coexist for a single User — Option-2 mode allows
// parallel tracking across Projects. Server-authoritative: clients POST to
// `/api/v1/active/<project-id>/start` and the server decides whether the
// start is allowed (rejected with 409 if another device holds it).
//
// StartedOnDevice is informational only; used by the conflict overlay to
// tell the user where the parallel session is running.
//
// ProjectName is a transient display field — never persisted. Set by
// usecase/TUI layers after a Projects store join so the indicator header
// shows human-readable names without N+1 lookups per session.
type ActiveSession struct {
	UserID      string
	ProjectID   string
	ProjectName string // transient: set by TUI activeSessionsListCmd enrich step
	StartedAt   time.Time
	// PausedAt is non-nil while the session is paused (server-set on
	// Pause, cleared on Resume). Nil = running.
	PausedAt *time.Time
	// PauseTotal accumulates completed pause intervals. It does NOT
	// include a currently-open pause — Elapsed() handles that.
	PauseTotal      time.Duration
	StartedOnDevice string
	Tag             string // Intent-Tag set at start; carried over to the finished Session on Stop unless overridden.
	Note            string // Free-text note set at start; merged into the finished Session on Stop.
	Version         int64  // Optimistic-Concurrency token, server-incremented
}

// Elapsed returns the worked duration at instant now: wall time since
// start minus completed pauses minus a currently-open pause. This is THE
// canonical elapsed formula (Spec §7 stop semantics) — every surface
// (API stop, WebUI banner, später TUI) MUST use it instead of computing
// now.Sub(StartedAt) by hand.
func (a ActiveSession) Elapsed(now time.Time) time.Duration {
	e := now.Sub(a.StartedAt) - a.PauseTotal
	if a.PausedAt != nil {
		e -= now.Sub(*a.PausedAt)
	}
	if e < 0 {
		return 0
	}
	return e
}
