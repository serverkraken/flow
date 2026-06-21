package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListTags aggregates the owner's document tags with per-tag counts.
type ListTags struct{ Docs ports.DocumentStore }

func (uc ListTags) Execute(ctx context.Context, ownerID string) ([]domain.TagCount, error) {
	docs, err := uc.Docs.List(ctx, ownerID, nil)
	if err != nil {
		return nil, err
	}
	return domain.CollectTags(docs), nil
}
