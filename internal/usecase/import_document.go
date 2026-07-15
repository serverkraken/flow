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
// corpus with its original identity. It still stamps id+timestamps, validates,
// and extracts wikilinks.
type ImportDocument struct {
	Docs     ports.DocumentStore
	Nodes    ports.NodeStore
	Tags     ports.TagStore // optional until B3 wires the composition root
	IDs      ports.IDGen
	Clock    ports.Clock
	Notifier ports.DocChangeNotifier // optional; nil → no notification
}

// ImportDocumentInput is the caller-supplied shape for a verbatim import.
type ImportDocumentInput struct {
	Type   domain.DocumentType
	Path   string
	Title  string
	Body   string
	Date   *time.Time
	NodeID *string
	Tags   []string // explicit tag set; nil → no tags
}

func (uc ImportDocument) Execute(ctx context.Context, ownerID string, in ImportDocumentInput) (domain.Document, error) {
	if err := requireOwnedNode(ctx, uc.Nodes, ownerID, in.NodeID); err != nil {
		return domain.Document{}, err
	}
	now := uc.Clock.Now()
	eff := in.Tags
	d := domain.Document{
		ID: uc.IDs.NewID(), OwnerID: ownerID, NodeID: in.NodeID, Type: in.Type,
		Path:  in.Path,
		Title: domain.StripHighlightSentinels(in.Title),
		Body:  domain.StripHighlightSentinels(in.Body),
		Tags:  domain.NormalizeTags(eff), // fake-store filter field only; pgstore reads tags from taggings (column dropped)
		Date:  in.Date, CreatedAt: now, UpdatedAt: now,
	}
	_, bodyStart := domain.ParseFrontmatter(d.Body)
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
