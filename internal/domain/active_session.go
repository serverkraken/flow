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
	UserID          string
	ProjectID       string
	ProjectName     string // transient: set by TUI activeSessionsListCmd enrich step
	StartedAt       time.Time
	StartedOnDevice string
	Tag             string // Intent-Tag set at start; carried over to the finished Session on Stop unless overridden.
	Note            string // Free-text note set at start; merged into the finished Session on Stop.
	Version         int64  // Optimistic-Concurrency token, server-incremented
}
