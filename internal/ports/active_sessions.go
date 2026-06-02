package ports

import "github.com/serverkraken/flow/internal/domain"

// ActiveSessionStore tracks in-progress worktime per (User, Project).
type ActiveSessionStore interface {
	ListByUser(userID string) ([]domain.ActiveSession, error)
	Get(userID, projectID string) (domain.ActiveSession, error)
	Upsert(a domain.ActiveSession) error
	Delete(userID, projectID string) error
}

// ErrActiveSessionNotFound is returned by ActiveSessionStore when there is no in-progress session for the given (user, project) pair.
var ErrActiveSessionNotFound = errSentinel("flow: active session not found")
