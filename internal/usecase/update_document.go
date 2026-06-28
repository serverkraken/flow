package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UpdateDocument edits the title and body of an owner's document. Tags may be
// supplied explicitly; when omitted, they are unchanged. Path/type/project are
// immutable in the spine.
type UpdateDocument struct {
	Docs     ports.DocumentStore
	Tags     ports.TagStore          // optional until B3 wires the composition root
	Clock    ports.Clock
	Notifier ports.DocChangeNotifier // optional; nil → no notification
}

type UpdateDocumentInput struct {
	Title string
	Body  string
	Tags  *[]string // nil → leave tags unchanged; non-nil (incl. empty) → replace
}

func (uc UpdateDocument) Execute(ctx context.Context, ownerID, id string, in UpdateDocumentInput) (domain.Document, error) {
	cur, err := uc.Docs.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Document{}, err
	}
	cur.Title, cur.Body = domain.StripHighlightSentinels(in.Title), domain.StripHighlightSentinels(in.Body)
	_, bodyStart := domain.ParseFrontmatter(in.Body)
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
	if uc.Tags != nil && in.Tags != nil {
		tags, err := uc.Tags.SetTags(ctx, ownerID, domain.TaggableDocument, updated.ID, *in.Tags)
		if err != nil {
			return updated, err
		}
		updated.Tags = slugsOf(tags)
	}
	if uc.Notifier != nil {
		uc.Notifier.DocumentChanged()
	}
	return updated, nil
}
