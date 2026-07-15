package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type DeleteDocument struct {
	Docs      ports.DocumentStore
	Aggregate ports.DocumentAggregateStore
	Tags      ports.TagStore // legacy fallback for callers without Aggregate
}

func (uc DeleteDocument) Execute(ctx context.Context, ownerID, id string) error {
	if uc.Aggregate != nil {
		return uc.Aggregate.DeleteDocumentAggregate(ctx, ownerID, id)
	}
	if err := uc.Docs.Delete(ctx, ownerID, id); err != nil {
		return err
	}
	if uc.Tags != nil {
		return uc.Tags.ClearTaggable(ctx, ownerID, domain.TaggableDocument, id)
	}
	return nil
}
