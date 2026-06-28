package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type GetTags struct{ Tags ports.TagStore }

func (uc GetTags) Execute(ctx context.Context, ownerID string, typ domain.TaggableType, id string) ([]domain.Tag, error) {
	return uc.Tags.TagsFor(ctx, ownerID, typ, id)
}
