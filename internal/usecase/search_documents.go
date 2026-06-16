package usecase

import (
	"context"
	"log/slog"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SearchDocuments runs a ranked keyword search (FTS + fuzzy) and, when an Embedder
// is configured and reachable, fuses it with a semantic (vector) arm via RRF.
// If the Embedder errors (e.g. Ollama down) the search degrades to keyword-only.
type SearchDocuments struct {
	Docs     ports.DocumentStore
	Embedder ports.Embedder // optional; nil → keyword-only
	Limit    int            // candidates per semantic arm; <=0 → 50
	Log      *slog.Logger   // optional
}

func (uc SearchDocuments) Execute(ctx context.Context, ownerID, q string, tags []string) ([]domain.SearchHit, error) {
	keyword, err := uc.Docs.Search(ctx, ownerID, q, tags)
	if err != nil {
		return nil, err
	}
	if uc.Embedder == nil {
		return keyword, nil
	}
	vecs, err := uc.Embedder.Embed(ctx, []string{q})
	if err != nil || len(vecs) == 0 {
		uc.warn("semantic search degraded; keyword-only", err)
		return keyword, nil
	}
	limit := uc.Limit
	if limit <= 0 {
		limit = 50
	}
	semantic, err := uc.Docs.SemanticSearch(ctx, ownerID, vecs[0], tags, limit)
	if err != nil {
		uc.warn("semantic search failed; keyword-only", err)
		return keyword, nil
	}
	return rrfFuse(keyword, semantic, rrfK), nil
}

func (uc SearchDocuments) warn(msg string, err error) {
	log := uc.Log
	if log == nil {
		log = slog.Default()
	}
	log.Warn(msg, "err", err)
}
