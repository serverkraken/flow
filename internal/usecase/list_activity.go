package usecase

import (
	"context"

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
