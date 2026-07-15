package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UpdateDocument edits the title and body of an owner's document. Tags may be
// supplied explicitly; when omitted, they are unchanged. This use case
// deliberately preserves the loaded type/path (only title/body/tags change
// here); Docs.Update itself can persist type/path for maintenance ops.
type UpdateDocument struct {
	Docs      ports.DocumentStore
	Aggregate ports.DocumentAggregateStore
	Tags      ports.TagStore // legacy fallback for callers without Aggregate
	Clock     ports.Clock
	Notifier  ports.DocChangeNotifier // optional; nil → no notification
}

type UpdateDocumentInput struct {
	Title string
	Body  string
	Tags  *[]string // nil → leave tags unchanged; non-nil (incl. empty) → replace
}

func (uc UpdateDocument) Execute(ctx context.Context, ownerID, id string, in UpdateDocumentInput) (domain.Document, error) {
	mutate := func(cur domain.Document) (domain.Document, ports.DocumentAggregateChanges, error) {
		cur.Title = domain.StripHighlightSentinels(in.Title)
		cur.Body = domain.StripHighlightSentinels(in.Body)
		_, bodyStart := domain.ParseFrontmatter(cur.Body)
		cur.UpdatedAt = uc.Clock.Now()
		a := actor.FromContext(ctx)
		cur.UpdatedByKind, cur.UpdatedByRef = string(a.Kind), a.Ref
		return cur, ports.DocumentAggregateChanges{
			Links: domain.WikilinkTargets(cur.Body[bodyStart:]),
			Tags:  in.Tags,
		}, nil
	}
	if uc.Aggregate != nil {
		updated, err := uc.Aggregate.UpdateDocumentAggregate(ctx, ownerID, id, mutate)
		if err != nil {
			return domain.Document{}, err
		}
		if uc.Notifier != nil {
			uc.Notifier.DocumentChanged()
		}
		return updated, nil
	}
	cur, err := uc.Docs.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Document{}, err
	}
	cur, changes, err := mutate(cur)
	if err != nil {
		return domain.Document{}, err
	}
	updated, err := uc.Docs.Update(ctx, cur)
	if err != nil {
		return domain.Document{}, err
	}
	// Legacy fallback for isolated callers that have not wired Aggregate.
	if err := uc.Docs.ReplaceLinks(ctx, updated.ID, ownerID, changes.Links); err != nil {
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
