// Package worker holds background workers. EmbedWorker keeps document embeddings
// up to date asynchronously so the write path never depends on Ollama.
package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/serverkraken/flow/internal/chunk"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// EmbedWorker periodically embeds stale documents. It also exposes DocumentChanged
// so writes can wake it promptly (ports.DocChangeNotifier).
type EmbedWorker struct {
	docs     ports.DocumentStore
	embedder ports.Embedder
	interval time.Duration
	batch    int
	kick     chan struct{}
	log      *slog.Logger
}

// NewEmbedWorker constructs the worker. interval <= 0 disables the periodic tick
// (used in tests that call drain directly).
func NewEmbedWorker(docs ports.DocumentStore, e ports.Embedder, interval time.Duration, batch int, log *slog.Logger) *EmbedWorker {
	if batch <= 0 {
		batch = 16
	}
	return &EmbedWorker{docs: docs, embedder: e, interval: interval, batch: batch, kick: make(chan struct{}, 1), log: log}
}

// DocumentChanged wakes the worker (non-blocking, coalesced). Implements
// ports.DocChangeNotifier.
func (w *EmbedWorker) DocumentChanged() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// Run loops until ctx is cancelled: backfill once, then react to ticks and kicks.
func (w *EmbedWorker) Run(ctx context.Context) {
	w.drain(ctx)
	if w.interval <= 0 {
		<-ctx.Done()
		return
	}
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.drain(ctx)
		case <-w.kick:
			w.drain(ctx)
		}
	}
}

// drain embeds stale documents until none remain or an embed error stops the cycle.
func (w *EmbedWorker) drain(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		docs, err := w.docs.StaleDocuments(ctx, w.batch)
		if err != nil {
			w.log.Warn("embed worker: list stale", "err", err)
			return
		}
		if len(docs) == 0 {
			return
		}
		for _, d := range docs {
			if ctx.Err() != nil {
				return
			}
			if err := w.embedOne(ctx, d); err != nil {
				w.log.Warn("embed worker: embed doc", "id", d.ID, "err", err)
				return // backend likely down; retry next tick
			}
		}
	}
}

func (w *EmbedWorker) embedOne(ctx context.Context, d domain.Document) error {
	texts := chunk.Split(d.Title, d.Body)
	if len(texts) == 0 {
		return w.docs.ReplaceChunks(ctx, d.ID, d.OwnerID, nil, nil)
	}
	vecs, err := w.embedder.Embed(ctx, texts)
	if err != nil {
		return err
	}
	return w.docs.ReplaceChunks(ctx, d.ID, d.OwnerID, texts, vecs)
}
