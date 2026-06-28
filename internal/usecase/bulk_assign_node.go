package usecase

import (
	"context"
	"errors"

	"github.com/serverkraken/flow/internal/ports"
)

// ErrNoSessions is returned by bulk operations when the id list is empty.
var ErrNoSessions = errors.New("no sessions selected")

// BulkAssignNode assigns one project to many sessions (import cleanup).
// Owner-scoped: the project must belong to the owner; sessions that are missing
// or foreign are silently skipped (robust against a stale selection). Start/stop
// are untouched, so no overlap check is needed. Returns the count actually changed.
type BulkAssignNode struct {
	Sessions ports.SessionStore
	Nodes	ports.NodeStore
}

func (uc BulkAssignNode) Execute(ctx context.Context, ownerID string, sessionIDs []string, nodeID string) (int, error) {
	if len(sessionIDs) == 0 {
		return 0, ErrNoSessions
	}
	// Validate the target project up front (owner-scoped).
	if _, err := uc.Nodes.Get(ctx, ownerID, nodeID); err != nil {
		return 0, err // ports.ErrNodeNotFound for missing/foreign
	}
	pid := nodeID
	updated := 0
	for _, id := range sessionIDs {
		cur, err := uc.Sessions.Get(ctx, ownerID, id)
		if errors.Is(err, ports.ErrSessionNotFound) {
			continue // stale/foreign — skip
		}
		if err != nil {
			return updated, err
		}
		// Tags live in the taggings junction keyed by session id, untouched by a
		// project reassignment, so this node-only Update preserves them implicitly.
		if _, err := uc.Sessions.Update(ctx, ownerID, id, &pid, cur.Note, cur.Start, cur.Stop); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}
