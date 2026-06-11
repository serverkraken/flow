package usecase

import (
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ActiveSessions orchestrates Start/Stop/Pause/Resume/CorrectStart by
// delegating synchronously to the WorktimeMachine (server-side state machine).
// All queue logic, local session writes, and conflict overlay are removed;
// the server is now the single source of truth.
type ActiveSessions struct {
	users    ports.UserStore // reserved for future callers (Task 32 wiring)
	projects ports.ProjectStore
	active   ports.ActiveSessionStore
	machine  ports.WorktimeMachine
}

// NewActiveSessions constructs an ActiveSessions use case.
// users may be nil until Task 32 wires the composition root.
func NewActiveSessions(
	users ports.UserStore,
	projects ports.ProjectStore,
	active ports.ActiveSessionStore,
	machine ports.WorktimeMachine,
) *ActiveSessions {
	return &ActiveSessions{
		users:    users,
		projects: projects,
		active:   active,
		machine:  machine,
	}
}

// Start begins a new active session on the server. On a 409 conflict the
// server returns ErrActiveSessionConflict which is mapped to
// ErrActiveSessionExists so callers receive a stable sentinel.
func (a *ActiveSessions) Start(userID, projectID, tag, note string) (domain.ActiveSession, error) {
	as, err := a.machine.Start(projectID, tag, note)
	if errors.Is(err, ports.ErrActiveSessionConflict) {
		return domain.ActiveSession{}, ErrActiveSessionExists
	}
	return as, err
}

// Stop ends the active session for projectID and returns the finished Session.
// tag and note are stored on the active-session row server-side at start time;
// the use case therefore ignores them (the server reads them from the row).
func (a *ActiveSessions) Stop(userID, projectID, tag, note string) (domain.Session, error) {
	return a.machine.Stop(projectID)
}

// Pause suspends the running session for projectID.
func (a *ActiveSessions) Pause(userID, projectID string) (domain.ActiveSession, error) {
	return a.machine.Pause(projectID)
}

// Resume restarts a previously paused session for projectID.
func (a *ActiveSessions) Resume(userID, projectID string) (domain.ActiveSession, error) {
	return a.machine.Resume(projectID)
}

// CorrectStart moves the start time of the user's earliest running session.
// Returns ports.ErrActiveSessionNotFound when nothing is running.
func (a *ActiveSessions) CorrectStart(userID string, ts time.Time) error {
	list, err := a.active.ListByUser(userID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return ports.ErrActiveSessionNotFound
	}
	// Find the earliest running session.
	cur := list[0]
	for _, c := range list[1:] {
		if c.StartedAt.Before(cur.StartedAt) {
			cur = c
		}
	}
	_, err = a.machine.CorrectStart(cur.ProjectID, ts)
	return err
}

// ListActive returns currently running sessions across all projects for the
// given user.
func (a *ActiveSessions) ListActive(userID string) ([]domain.ActiveSession, error) {
	return a.active.ListByUser(userID)
}

// ErrActiveSessionExists is returned by Start when the server reports a 409
// conflict for the given (userID, projectID) pair.
var ErrActiveSessionExists = errors.New("flow: active session for this project already exists")
