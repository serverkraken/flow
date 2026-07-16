package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

const MaxBulkDocumentCuration = 200

type BulkCurateDocuments struct {
	Docs  ports.DocumentCurationStore
	Clock ports.Clock
}

type BulkCurateDocumentsInput struct {
	IDs         []string
	Archived    *bool
	ContextMode *domain.ContextMode
}

// Execute validates and de-duplicates one bounded selection before handing it
// to the store's all-or-nothing transaction. Events remain an adapter concern
// and may only be emitted after Execute returns successfully.
func (uc BulkCurateDocuments) Execute(ctx context.Context, ownerID string, in BulkCurateDocumentsInput) ([]domain.Document, error) {
	ids := normalizedDocumentIDs(in.IDs)
	actions := 0
	if in.Archived != nil {
		actions++
	}
	if in.ContextMode != nil {
		actions++
	}
	if ownerID == "" || len(ids) == 0 || len(ids) > MaxBulkDocumentCuration || actions != 1 {
		return nil, fmt.Errorf("%w: invalid bulk curation", domain.ErrInvalidDocument)
	}
	if in.ContextMode != nil && !in.ContextMode.Valid() {
		return nil, fmt.Errorf("%w: bad context mode %q", domain.ErrInvalidDocument, *in.ContextMode)
	}
	a := actor.FromContext(ctx)
	return uc.Docs.CurateDocuments(ctx, ownerID, ports.DocumentCurationMutation{
		IDs: ids, Archived: in.Archived, ContextMode: in.ContextMode,
		ActorKind: string(a.Kind), ActorRef: a.Ref, At: uc.Clock.Now(),
	})
}

func normalizedDocumentIDs(raw []string) []string {
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
