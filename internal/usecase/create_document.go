package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// CreateDocument stamps id+timestamps, derives the daily path from the date,
// validates, and persists an owner-scoped document.
type CreateDocument struct {
	Docs     ports.DocumentStore
	IDs      ports.IDGen
	Clock    ports.Clock
	Notifier ports.DocChangeNotifier // optional; nil → no notification
}

// CreateDocumentInput is the caller-supplied shape (the use case fills the rest).
type CreateDocumentInput struct {
	Type      domain.DocumentType
	ProjectID *string
	Path      string
	Title     string
	Body      string
}

func (uc CreateDocument) Execute(ctx context.Context, ownerID string, in CreateDocumentInput) (domain.Document, error) {
	now := uc.Clock.Now()
	d := domain.Document{
		ID: uc.IDs.NewID(), OwnerID: ownerID, ProjectID: in.ProjectID, Type: in.Type,
		Path: in.Path, Title: domain.StripHighlightSentinels(in.Title), Body: domain.StripHighlightSentinels(in.Body),
		CreatedAt: now, UpdatedAt: now,
	}
	if in.Type == domain.DocDaily {
		d.Date = &now
		d.Path = domain.DailyPath(now)
	}
	tags, bodyStart := domain.ParseFrontmatter(d.Body)
	d.Tags = tags
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
	if uc.Notifier != nil {
		uc.Notifier.DocumentChanged()
	}
	return created, nil
}
