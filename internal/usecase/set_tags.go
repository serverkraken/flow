package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type SetTags struct{ Tags ports.TagStore }

func (uc SetTags) Execute(ctx context.Context, ownerID string, typ domain.TaggableType, id string, raw []string) ([]domain.Tag, error) {
	return uc.Tags.SetTags(ctx, ownerID, typ, id, raw)
}
