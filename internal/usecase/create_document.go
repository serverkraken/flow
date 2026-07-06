package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// CreateDocument stamps id+timestamps, derives the daily path from the date,
// validates, and persists an owner-scoped document.
type CreateDocument struct {
	Docs     ports.DocumentStore
	Tags     ports.TagStore          // optional until B3 wires the composition root
	IDs      ports.IDGen
	Clock    ports.Clock
	Notifier ports.DocChangeNotifier // optional; nil → no notification
}

// CreateDocumentInput is the caller-supplied shape (the use case fills the rest).
type CreateDocumentInput struct {
	Type   domain.DocumentType
	NodeID *string
	Path   string
	Title  string
	Body   string
	Tags   []string // explicit tag set; nil → no tags
}

func (uc CreateDocument) Execute(ctx context.Context, ownerID string, in CreateDocumentInput) (domain.Document, error) {
	now := uc.Clock.Now()
	eff := in.Tags
	a := actor.FromContext(ctx)
	d := domain.Document{
		ID: uc.IDs.NewID(), OwnerID: ownerID, NodeID: in.NodeID, Type: in.Type,
		Path: in.Path, Title: domain.StripHighlightSentinels(in.Title), Body: domain.StripHighlightSentinels(in.Body),
		Tags:      domain.NormalizeTags(eff), // fake-store filter field only; pgstore reads tags from taggings (column dropped)
		CreatedAt: now, UpdatedAt: now,
		UpdatedByKind: string(a.Kind), UpdatedByRef: a.Ref,
	}
	if in.Type == domain.DocDaily {
		d.Date = &now
		d.Path = domain.DailyPath(now)
	}
	_, bodyStart := domain.ParseFrontmatter(d.Body)
	if err := d.Validate(); err != nil {
		return domain.Document{}, err
	}
	created, err := uc.Docs.Create(ctx, d)
	if err != nil {
		return domain.Document{}, err
	}
	// Link extraction is deliberately non-atomic: the document is already
	// persisted above. A ReplaceLinks failure surfaces as an error even though
	// the create succeeded; a subsequent save heals the link index.
	if err := uc.Docs.ReplaceLinks(ctx, created.ID, ownerID, domain.WikilinkTargets(created.Body[bodyStart:])); err != nil {
		return domain.Document{}, err
	}
	if uc.Tags != nil {
		tags, err := uc.Tags.SetTags(ctx, ownerID, domain.TaggableDocument, created.ID, eff)
		if err != nil {
			return created, err
		}
		created.Tags = slugsOf(tags)
	}
	if uc.Notifier != nil {
		uc.Notifier.DocumentChanged()
	}
	return created, nil
}
