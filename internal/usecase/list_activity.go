package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type ListActivity struct{ Activities ports.ActivityStore }

func (uc ListActivity) Execute(ctx context.Context, ownerID string, classes []string, actorRef *string, limit, offset int) ([]domain.ActivityEntry, int, error) {
	return uc.Activities.ListPage(ctx, ownerID, classes, actorRef, limit, offset)
}

// Actors returns all distinct actor_refs for the owner, independent of any
// filter, so UI dropdowns always show the full set.
func (uc ListActivity) Actors(ctx context.Context, ownerID string) ([]string, error) {
	return uc.Activities.DistinctActors(ctx, ownerID)
}

// ForNodes returns one page of the owner's activity restricted to a node set
// — the register entry point's feed. It is a distinct method rather than a
// filter on Execute because the promise differs: Execute pages the owner's
// activity, ForNodes pages THIS register's, and only the store can keep that
// promise (Soenne, 21.08.).
func (uc ListActivity) ForNodes(ctx context.Context, ownerID string, nodeIDs []string, limit, offset int) ([]domain.ActivityEntry, int, error) {
	return uc.Activities.ListPageForNodes(ctx, ownerID, nodeIDs, limit, offset)
}

// AgentsSince counts the distinct agent actors that touched the node set at or
// after since — the register head's "N Agenten heute aktiv".
func (uc ListActivity) AgentsSince(ctx context.Context, ownerID string, nodeIDs []string, since time.Time) (int, error) {
	return uc.Activities.DistinctAgentsSince(ctx, ownerID, nodeIDs, since)
}
