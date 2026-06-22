package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// GetEmbedStatus returns the owner-scoped embedding status of one document.
type GetEmbedStatus struct {
	Docs ports.DocumentStore
}

// Execute returns the document's embed status (ErrDocumentNotFound if unknown).
func (uc GetEmbedStatus) Execute(ctx context.Context, ownerID, docID string) (domain.EmbedStatus, error) {
	return uc.Docs.EmbedStatus(ctx, ownerID, docID)
}
