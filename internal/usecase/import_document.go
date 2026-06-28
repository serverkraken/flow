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
	Tags     ports.TagStore          // optional until B3 wires the composition root
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
	Tags   []string // explicit tag set; nil → derive from YAML frontmatter (B1 fallback)
}

func (uc ImportDocument) Execute(ctx context.Context, ownerID string, in ImportDocumentInput) (domain.Document, error) {
	now := uc.Clock.Now()
	d := domain.Document{
		ID: uc.IDs.NewID(), OwnerID: ownerID, NodeID: in.NodeID, Type: in.Type,
		Path:      in.Path,
		Title:     domain.StripHighlightSentinels(in.Title),
		Body:      domain.StripHighlightSentinels(in.Body),
		Date:      in.Date, CreatedAt: now, UpdatedAt: now,
	}
	_, bodyStart := domain.ParseFrontmatter(d.Body)
	effImport := in.Tags
	if effImport == nil { // B1 fallback: legacy frontmatter still wins when no explicit tags given
		effImport, _ = domain.ParseFrontmatter(d.Body)
	}
	d.Tags = domain.NormalizeTags(effImport) // legacy column double-write (removed in B2)
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
		tags, err := uc.Tags.SetTags(ctx, ownerID, domain.TaggableDocument, created.ID, effImport)
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
