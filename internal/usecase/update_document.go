package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UpdateDocument edits title/body/tags of an owner's document. Path/type/project
// are immutable in the spine.
type UpdateDocument struct {
	Docs  ports.DocumentStore
	Clock ports.Clock
}

type UpdateDocumentInput struct {
	Title string
	Body  string
	Tags  []string
}

func (uc UpdateDocument) Execute(ctx context.Context, ownerID, id string, in UpdateDocumentInput) (domain.Document, error) {
	cur, err := uc.Docs.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Document{}, err
	}
	cur.Title, cur.Body, cur.Tags = in.Title, in.Body, in.Tags
	cur.UpdatedAt = uc.Clock.Now()
	return uc.Docs.Update(ctx, cur)
}
