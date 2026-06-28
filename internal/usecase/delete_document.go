package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type DeleteDocument struct {
	Docs ports.DocumentStore
	Tags ports.TagStore // optional; if non-nil, taggings are cleared on delete
}

func (uc DeleteDocument) Execute(ctx context.Context, ownerID, id string) error {
	if err := uc.Docs.Delete(ctx, ownerID, id); err != nil {
		return err
	}
	if uc.Tags != nil {
		return uc.Tags.ClearTaggable(ctx, ownerID, domain.TaggableDocument, id)
	}
	return nil
}
