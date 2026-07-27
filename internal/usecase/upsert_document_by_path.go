package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UpsertByPathInput is the caller-supplied shape for an idempotent upsert keyed
// by (owner, node, path). Used by the memory-migration importer.
type UpsertByPathInput struct {
	Type     domain.DocumentType
	NodeID   *string
	Path     string
	Title    string
	Body     string
	Tags     []string // explicit tag set; nil -> leave tags untouched
	Pinned   bool
	Archived bool
}

// UpsertDocumentByPath inserts or updates a document at (owner, node, path),
// re-extracts wikilinks, applies the tag set, and enforces the pinned flag on
// every run (so re-imports stay in sync). It is the general idempotent write
// behind `flow context migrate memories`.
type UpsertDocumentByPath struct {
	Docs      ports.DocumentStore
	Aggregate ports.DocumentAggregateStore
	Nodes     ports.NodeStore
	Tags      ports.TagStore
	Notifier  ports.DocChangeNotifier // optional; nil -> no notification
}

func (uc UpsertDocumentByPath) Execute(ctx context.Context, ownerID string, in UpsertByPathInput) (string, time.Time, error) {
	if err := requireOwnedNode(ctx, uc.Nodes, ownerID, in.NodeID); err != nil {
		return "", time.Time{}, err
	}
	// Validate via a domain.Document (type set, slug form, project rule).
	if err := (domain.Document{Type: in.Type, NodeID: in.NodeID, Path: in.Path, Title: in.Title, Body: in.Body}).Validate(); err != nil {
		return "", time.Time{}, err
	}
	a := actor.FromContext(ctx)
	if uc.Aggregate != nil {
		var tags *[]string
		if in.Tags != nil {
			tagValues := in.Tags
			tags = &tagValues
		}
		doc, err := uc.Aggregate.UpsertDocumentAggregate(ctx, ports.DocumentAggregateUpsert{
			OwnerID:       ownerID,
			NodeID:        in.NodeID,
			Type:          in.Type,
			Path:          in.Path,
			Title:         in.Title,
			Body:          in.Body,
			Pinned:        in.Pinned,
			Archived:      in.Archived,
			UpdatedByKind: string(a.Kind),
			UpdatedByRef:  a.Ref,
			Changes: ports.DocumentAggregateChanges{
				Links: domain.WikilinkTargets(in.Body),
				Tags:  tags,
			},
		})
		if err != nil {
			return "", time.Time{}, err
		}
		if uc.Notifier != nil {
			uc.Notifier.DocumentChanged()
		}
		return doc.ID, doc.UpdatedAt, nil
	}
	id, updated, err := uc.Docs.UpsertByPath(ctx, ownerID, in.NodeID, in.Type, in.Path, in.Title, in.Body, in.Pinned, in.Archived, string(a.Kind), a.Ref)
	if err != nil {
		return "", time.Time{}, err
	}
	if err := uc.Docs.ReplaceLinks(ctx, id, ownerID, domain.WikilinkTargets(in.Body)); err != nil {
		return "", time.Time{}, err
	}
	if uc.Tags != nil && in.Tags != nil {
		if _, err := uc.Tags.SetTags(ctx, ownerID, domain.TaggableDocument, id, in.Tags); err != nil {
			return id, updated, err
		}
	}
	// Legacy fallback enforces curation fields in separate store calls.
	if err := uc.Docs.SetPinned(ctx, ownerID, id, in.Pinned); err != nil {
		return id, updated, err
	}
	if err := uc.Docs.SetArchived(ctx, ownerID, id, in.Archived); err != nil {
		return "", time.Time{}, fmt.Errorf("upsert by path: set archived: %w", err)
	}
	if uc.Notifier != nil {
		uc.Notifier.DocumentChanged()
	}
	return id, updated, nil
}
