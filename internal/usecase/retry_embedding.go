// Package usecase — RetryEmbedding re-queues a document for embedding by
// clearing its recorded failure (incl. a dead-letter) and waking the worker.
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

// RetryEmbedding clears a document's embed-failure state and kicks the worker.
type RetryEmbedding struct {
	Docs     ports.DocumentStore
	Notifier ports.DocChangeNotifier // optional; nil → no kick
}

// Execute verifies ownership, clears the failure row, and notifies the worker.
func (uc RetryEmbedding) Execute(ctx context.Context, ownerID, docID string) error {
	if _, err := uc.Docs.Get(ctx, ownerID, docID); err != nil {
		return err // ErrDocumentNotFound for unknown/forbidden
	}
	if err := uc.Docs.ClearEmbedFailure(ctx, docID, ownerID); err != nil {
		return err
	}
	if uc.Notifier != nil {
		uc.Notifier.DocumentChanged()
	}
	return nil
}
