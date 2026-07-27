package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

const MaxBulkSessions = 200

var (
	// ErrNoSessions is returned by bulk operations when the id list is empty.
	ErrNoSessions = errors.New("no sessions selected")
	// ErrTooManySessions keeps every session bulk operation bounded per request.
	ErrTooManySessions = errors.New("too many sessions selected")
)

func normalizedSessionIDs(raw []string) ([]string, error) {
	if len(raw) > MaxBulkSessions {
		return nil, ErrTooManySessions
	}
	seen := make(map[string]bool, len(raw))
	ids := make([]string, 0, len(raw))
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, ErrNoSessions
	}
	return ids, nil
}

// BulkAssignNode assigns one project to many sessions (import cleanup).
// Owner-scoped: the project must belong to the owner; sessions that are missing
// or foreign are silently skipped (robust against a stale selection). Start/stop
// are untouched, so no overlap check is needed. Returns the count actually changed.
type BulkAssignNode struct {
	Sessions ports.SessionStore
	Nodes    ports.NodeStore
}

func (uc BulkAssignNode) Execute(ctx context.Context, ownerID string, sessionIDs []string, nodeID string) (int, error) {
	var err error
	sessionIDs, err = normalizedSessionIDs(sessionIDs)
	if err != nil {
		return 0, err
	}
	// Validate the target project up front (owner-scoped).
	n, err := uc.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return 0, err // ports.ErrNodeNotFound for missing/foreign
	}
	if !domain.IsBookable(n.Kind) {
		return 0, fmt.Errorf("%w: worktime books to a bookable node, got %s", domain.ErrInvalidNode, n.Kind)
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
