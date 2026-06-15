package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// CreateDocument stamps id+timestamps, derives the daily path from the date,
// validates, and persists an owner-scoped document.
type CreateDocument struct {
	Docs  ports.DocumentStore
	IDs   ports.IDGen
	Clock ports.Clock
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
		Path: in.Path, Title: in.Title, Body: in.Body, CreatedAt: now, UpdatedAt: now,
	}
	if in.Type == domain.DocDaily {
		d.Date = &now
		d.Path = domain.DailyPath(now)
	}
	if err := d.Validate(); err != nil {
		return domain.Document{}, err
	}
	return uc.Docs.Create(ctx, d)
}
