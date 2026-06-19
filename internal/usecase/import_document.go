package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ImportDocument persists a document verbatim: it honours the caller's path,
// type, date and project (unlike CreateDocument, which stamps daily docs with
// today's date and the canonical daily path). Used to re-home an existing
// corpus with its original identity. It still stamps id+timestamps, derives
// tags from frontmatter, validates, and extracts wikilinks.
type ImportDocument struct {
	Docs     ports.DocumentStore
	IDs      ports.IDGen
	Clock    ports.Clock
	Notifier ports.DocChangeNotifier // optional; nil → no notification
}

// ImportDocumentInput is the caller-supplied shape for a verbatim import.
type ImportDocumentInput struct {
	Type      domain.DocumentType
	Path      string
	Title     string
	Body      string
	Date      *time.Time
	ProjectID *string
}

func (uc ImportDocument) Execute(ctx context.Context, ownerID string, in ImportDocumentInput) (domain.Document, error) {
	now := uc.Clock.Now()
	d := domain.Document{
		ID: uc.IDs.NewID(), OwnerID: ownerID, ProjectID: in.ProjectID, Type: in.Type,
		Path:      in.Path,
		Title:     domain.StripHighlightSentinels(in.Title),
		Body:      domain.StripHighlightSentinels(in.Body),
		Date:      in.Date, CreatedAt: now, UpdatedAt: now,
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
	if err := uc.Docs.ReplaceLinks(ctx, created.ID, ownerID, domain.WikilinkTargets(created.Body[bodyStart:])); err != nil {
		return domain.Document{}, err
	}
	if uc.Notifier != nil {
		uc.Notifier.DocumentChanged()
	}
	return created, nil
}
