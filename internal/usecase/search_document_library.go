package usecase

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// SearchDocumentLibrary embeds the query when possible and delegates the
// complete keyword/semantic fusion, status facets and pagination to one store
// read. Embedding failures deliberately degrade to keyword-only search.
type SearchDocumentLibrary struct {
	Docs              ports.DocumentStore
	Embedder          ports.Embedder
	QueryEmbedTimeout time.Duration
	Log               *slog.Logger
}

func (uc SearchDocumentLibrary) Execute(ctx context.Context, ownerID, search string, query ports.DocumentLibraryQuery) (ports.DocumentLibraryPage, error) {
	query.Search = strings.TrimSpace(search)
	if query.Search == "" || uc.Embedder == nil {
		return uc.Docs.ListLibraryPage(ctx, ownerID, query)
	}
	timeout := uc.QueryEmbedTimeout
	if timeout <= 0 {
		timeout = defaultQueryEmbedTimeout
	}
	embedCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	vecs, err := uc.Embedder.Embed(embedCtx, []string{query.Search})
	if err != nil || len(vecs) == 0 {
		uc.warn("library semantic search degraded; keyword-only", err)
		return uc.Docs.ListLibraryPage(ctx, ownerID, query)
	}
	query.Embedding = vecs[0]
	return uc.Docs.ListLibraryPage(ctx, ownerID, query)
}

func (uc SearchDocumentLibrary) warn(msg string, err error) {
	log := uc.Log
	if log == nil {
		log = slog.Default()
	}
	log.Warn(msg, "err", err)
}
