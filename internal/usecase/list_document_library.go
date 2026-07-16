package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

// ListDocumentLibrary exposes the bounded management query without leaking
// adapter-specific SQL concerns into the HTTP layer.
type ListDocumentLibrary struct{ Docs ports.DocumentStore }

func (uc ListDocumentLibrary) Execute(ctx context.Context, ownerID string, query ports.DocumentLibraryQuery) (ports.DocumentLibraryPage, error) {
	return uc.Docs.ListLibraryPage(ctx, ownerID, query)
}
