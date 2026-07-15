package usecase

import (
	"context"
	"fmt"
	"time"

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
	title, body := in.Title, in.Body
	return uc.execute(ctx, ownerID, id, PatchDocumentInput{Title: &title, Body: &body, Tags: in.Tags})
}

// PatchDocumentInput changes only explicitly supplied fields. ExpectedUpdatedAt
// is checked after the owner-scoped row is locked, so concurrent writers cannot
// pass the same precondition and silently overwrite one another.
type PatchDocumentInput struct {
	Title             *string
	Body              *string
	Tags              *[]string
	ExpectedUpdatedAt *time.Time
}

func (uc UpdateDocument) ExecutePatch(ctx context.Context, ownerID, id string, in PatchDocumentInput) (domain.Document, error) {
	if in.Title == nil && in.Body == nil && in.Tags == nil {
		return domain.Document{}, fmt.Errorf("%w: no document fields supplied", domain.ErrInvalidDocument)
	}
	return uc.execute(ctx, ownerID, id, in)
}

func (uc UpdateDocument) execute(ctx context.Context, ownerID, id string, in PatchDocumentInput) (domain.Document, error) {
	mutate := func(cur domain.Document) (domain.Document, ports.DocumentAggregateChanges, error) {
		if in.ExpectedUpdatedAt != nil && !cur.UpdatedAt.Equal(*in.ExpectedUpdatedAt) {
			return domain.Document{}, ports.DocumentAggregateChanges{}, ports.DocumentConflictError{CurrentUpdatedAt: cur.UpdatedAt}
		}
		if in.Title != nil {
			cur.Title = domain.StripHighlightSentinels(*in.Title)
		}
		if in.Body != nil {
			cur.Body = domain.StripHighlightSentinels(*in.Body)
		}
		_, bodyStart := domain.ParseFrontmatter(cur.Body)
		updatedAt := uc.Clock.Now()
		// updated_at doubles as the external CAS version. Keep it strictly
		// increasing even when two writes land within the clock's resolution.
		if !updatedAt.After(cur.UpdatedAt) {
			updatedAt = cur.UpdatedAt.Add(time.Microsecond)
		}
		cur.UpdatedAt = updatedAt
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
