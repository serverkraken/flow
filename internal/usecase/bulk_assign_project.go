package usecase

import (
	"context"
	"errors"

	"github.com/serverkraken/flow/internal/ports"
)

// ErrNoSessions is returned by bulk operations when the id list is empty.
var ErrNoSessions = errors.New("no sessions selected")

// BulkAssignProject assigns one project to many sessions (import cleanup).
// Owner-scoped: the project must belong to the owner; sessions that are missing
// or foreign are silently skipped (robust against a stale selection). Start/stop
// are untouched, so no overlap check is needed. Returns the count actually changed.
type BulkAssignProject struct {
	Sessions ports.SessionStore
	Projects ports.ProjectStore
}

func (uc BulkAssignProject) Execute(ctx context.Context, ownerID string, sessionIDs []string, projectID string) (int, error) {
	if len(sessionIDs) == 0 {
		return 0, ErrNoSessions
	}
	// Validate the target project up front (owner-scoped).
	if _, err := uc.Projects.Get(ctx, ownerID, projectID); err != nil {
		return 0, err // ports.ErrProjectNotFound for missing/foreign
	}
	pid := projectID
	updated := 0
	for _, id := range sessionIDs {
		cur, err := uc.Sessions.Get(ctx, ownerID, id)
		if errors.Is(err, ports.ErrSessionNotFound) {
			continue // stale/foreign — skip
		}
		if err != nil {
			return updated, err
		}
		if _, err := uc.Sessions.Update(ctx, ownerID, id, &pid, cur.Tag, cur.Note, cur.Start, cur.Stop); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}
