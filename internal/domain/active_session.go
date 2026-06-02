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
type ActiveSession struct {
	UserID          string
	ProjectID       string
	StartedAt       time.Time
	StartedOnDevice string
	Version         int64 // Optimistic-Concurrency token, server-incremented
}
