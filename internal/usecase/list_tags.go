package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListTags returns tag counts from the registry, optionally filtered by taggable type.
type ListTags struct{ Tags ports.TagStore }

func (uc ListTags) Execute(ctx context.Context, ownerID string, scope domain.TagScope) ([]domain.TagCount, error) {
	return uc.Tags.ListTags(ctx, ownerID, scope)
}
