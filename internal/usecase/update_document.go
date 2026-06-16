package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UpdateDocument edits the title and body of an owner's document; tags are
// derived from the body's frontmatter, not taken as input. Path/type/project
// are immutable in the spine.
type UpdateDocument struct {
	Docs  ports.DocumentStore
	Clock ports.Clock
}

type UpdateDocumentInput struct {
	Title string
	Body  string
}

func (uc UpdateDocument) Execute(ctx context.Context, ownerID, id string, in UpdateDocumentInput) (domain.Document, error) {
	cur, err := uc.Docs.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Document{}, err
	}
	cur.Title, cur.Body = in.Title, in.Body
	tags, bodyStart := domain.ParseFrontmatter(in.Body)
	cur.Tags = tags
	cur.UpdatedAt = uc.Clock.Now()
	updated, err := uc.Docs.Update(ctx, cur)
	if err != nil {
		return domain.Document{}, err
	}
	// Link extraction is deliberately non-atomic: the update is already
	// persisted above. A ReplaceLinks failure surfaces as an error even though
	// the update succeeded; a subsequent save heals the link index.
	if err := uc.Docs.ReplaceLinks(ctx, updated.ID, ownerID, domain.WikilinkTargets(updated.Body[bodyStart:])); err != nil {
		return domain.Document{}, err
	}
	return updated, nil
}
