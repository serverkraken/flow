package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// MoveDocument changes a document's complete classification/location in one
// store operation. Content, tags and curation metadata remain unchanged.
type MoveDocument struct {
	Docs     ports.DocumentStore
	Nodes    ports.NodeStore
	Clock    ports.Clock
	Notifier ports.DocChangeNotifier
}

type MoveDocumentInput struct {
	Type   domain.DocumentType
	NodeID *string
	Path   string
	Date   *time.Time
}

func (uc MoveDocument) Execute(ctx context.Context, ownerID, id string, in MoveDocumentInput) (domain.Document, error) {
	if err := requireOwnedNode(ctx, uc.Nodes, ownerID, in.NodeID); err != nil {
		return domain.Document{}, err
	}
	cur, err := uc.Docs.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Document{}, err
	}
	next, err := domain.ReclassifyDocumentMetadata(cur, domain.DocumentMetadata{
		Type: in.Type, NodeID: in.NodeID, Path: in.Path, Date: in.Date,
	})
	if err != nil {
		return domain.Document{}, err
	}
	next.UpdatedAt = uc.Clock.Now()
	a := actor.FromContext(ctx)
	next.UpdatedByKind, next.UpdatedByRef = string(a.Kind), a.Ref
	moved, err := uc.Docs.Move(ctx, next)
	if err != nil {
		return domain.Document{}, err
	}
	if uc.Notifier != nil {
		uc.Notifier.DocumentChanged()
	}
	return moved, nil
}
