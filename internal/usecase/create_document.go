package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// CreateDocument stamps id+timestamps, derives the daily path from the date,
// validates, and persists an owner-scoped document.
type CreateDocument struct {
	Docs      ports.DocumentStore
	Aggregate ports.DocumentAggregateStore
	Nodes     ports.NodeStore
	Tags      ports.TagStore // legacy fallback for callers without Aggregate
	IDs       ports.IDGen
	Clock     ports.Clock
	Notifier  ports.DocChangeNotifier // optional; nil → no notification
}

// CreateDocumentInput is the caller-supplied shape (the use case fills the rest).
type CreateDocumentInput struct {
	Type   domain.DocumentType
	NodeID *string
	Path   string
	Date   *time.Time
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
	date := in.Date
	if in.Type == domain.DocDaily && date == nil {
		date = &now
	}
	d, err := domain.ReclassifyDocumentMetadata(d, domain.DocumentMetadata{
		Type: in.Type, NodeID: in.NodeID, Path: in.Path, Date: date,
	})
	if err != nil {
		return domain.Document{}, err
	}
	if err := requireOwnedNode(ctx, uc.Nodes, ownerID, d.NodeID); err != nil {
		return domain.Document{}, err
	}
	_, bodyStart := domain.ParseFrontmatter(d.Body)
	links := domain.WikilinkTargets(d.Body[bodyStart:])
	if uc.Aggregate != nil {
		tags := eff
		created, err := uc.Aggregate.CreateDocumentAggregate(ctx, d, ports.DocumentAggregateChanges{
			Links: links,
			Tags:  &tags,
		})
		if err != nil {
			return domain.Document{}, err
		}
		if uc.Notifier != nil {
			uc.Notifier.DocumentChanged()
		}
		return created, nil
	}
	created, err := uc.Docs.Create(ctx, d)
	if err != nil {
		return domain.Document{}, err
	}
	// Legacy fallback for isolated callers that have not wired Aggregate.
	if err := uc.Docs.ReplaceLinks(ctx, created.ID, ownerID, links); err != nil {
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
