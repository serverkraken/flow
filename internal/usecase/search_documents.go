package usecase

import (
	"context"
	"log/slog"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// defaultQueryEmbedTimeout bounds the query-embed call so search degrades to
// keyword-only quickly when the embed backend is slow or down.
const defaultQueryEmbedTimeout = 3 * time.Second

// SearchDocuments runs a ranked keyword search (FTS + fuzzy) and, when an Embedder
// is configured and reachable, fuses it with a semantic (vector) arm via RRF.
// If the Embedder errors (e.g. Ollama down) the search degrades to keyword-only.
type SearchDocuments struct {
	Docs              ports.DocumentStore
	Embedder          ports.Embedder // optional; nil → keyword-only
	Limit             int            // candidates per semantic arm; <=0 → 50
	QueryEmbedTimeout time.Duration  // <=0 → defaultQueryEmbedTimeout
	Log               *slog.Logger   // optional
}

func (uc SearchDocuments) Execute(ctx context.Context, ownerID, q string, projectID *string, tags []string) ([]domain.SearchHit, error) {
	keyword, err := uc.Docs.Search(ctx, ownerID, q, projectID, tags)
	if err != nil {
		return nil, err
	}
	if uc.Embedder == nil {
		return keyword, nil
	}
	timeout := uc.QueryEmbedTimeout
	if timeout <= 0 {
		timeout = defaultQueryEmbedTimeout
	}
	embedCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	vecs, err := uc.Embedder.Embed(embedCtx, []string{q})
	if err != nil || len(vecs) == 0 {
		uc.warn("semantic search degraded; keyword-only", err)
		return keyword, nil
	}
	limit := uc.Limit
	if limit <= 0 {
		limit = 50
	}
	semantic, err := uc.Docs.SemanticSearch(ctx, ownerID, vecs[0], projectID, tags, limit)
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
