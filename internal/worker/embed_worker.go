// Package worker holds background workers. EmbedWorker keeps document embeddings
// up to date asynchronously so the write path never depends on Ollama.
package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/serverkraken/flow/internal/chunk"
	"github.com/serverkraken/flow/internal/ports"
)

const (
	defaultMaxAttempts = 5
	defaultBackoffBase = time.Minute
	defaultBackoffCap  = 6 * time.Hour
)

// EmbedPolicy tunes per-document failure handling. Zero fields take the defaults.
type EmbedPolicy struct {
	MaxAttempts int
	BackoffBase time.Duration
	BackoffCap  time.Duration
}

// EmbedWorker periodically embeds stale documents. It also exposes DocumentChanged
// so writes can wake it promptly (ports.DocChangeNotifier).
type EmbedWorker struct {
	docs     ports.DocumentStore
	embedder ports.Embedder
	interval time.Duration
	batch    int
	pol      EmbedPolicy
	clock    func() time.Time
	kick     chan struct{}
	log      *slog.Logger
}

// NewEmbedWorker constructs the worker. interval <= 0 disables the periodic tick
// (used in tests that call drain directly).
func NewEmbedWorker(docs ports.DocumentStore, e ports.Embedder, interval time.Duration, batch int, pol EmbedPolicy, log *slog.Logger) *EmbedWorker {
	if batch <= 0 {
		batch = 16
	}
	if pol.MaxAttempts <= 0 {
		pol.MaxAttempts = defaultMaxAttempts
	}
	if pol.BackoffBase <= 0 {
		pol.BackoffBase = defaultBackoffBase
	}
	if pol.BackoffCap <= 0 {
		pol.BackoffCap = defaultBackoffCap
	}
	return &EmbedWorker{docs: docs, embedder: e, interval: interval, batch: batch, pol: pol, clock: time.Now, kick: make(chan struct{}, 1), log: log}
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

// drain embeds stale documents until none remain or embedDoc signals a stop.
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
		for _, sd := range docs {
			if ctx.Err() != nil {
				return
			}
			if !w.embedDoc(ctx, sd) {
				return // transient backend failure or store error: stop, retry next tick
			}
		}
	}
}

// embedDoc embeds one document. It returns false when the drain must STOP (a
// transient backend failure or a store error — retry the whole batch next tick)
// and true when it may CONTINUE to the next document (success, or a per-doc
// failure that was recorded for backoff/dead-letter).
func (w *EmbedWorker) embedDoc(ctx context.Context, sd ports.StaleDoc) bool {
	d := sd.Doc
	texts := chunk.Split(d.Title, d.Body)
	if len(texts) == 0 {
		if err := w.docs.ReplaceChunks(ctx, d.ID, d.OwnerID, nil, nil); err != nil {
			w.log.Warn("embed worker: clear chunks", "id", d.ID, "err", err)
			return false
		}
		return true
	}
	vecs, err := w.embedder.Embed(ctx, texts)
	if err != nil {
		if errors.Is(err, ports.ErrEmbedTransient) {
			w.log.Warn("embed worker: backend unavailable", "id", d.ID, "err", err)
			return false // do not penalize the doc
		}
		attempts := sd.Attempts + 1
		dead := attempts >= w.pol.MaxAttempts
		next := w.clock().Add(backoff(attempts, w.pol.BackoffBase, w.pol.BackoffCap))
		if rerr := w.docs.RecordEmbedFailure(ctx, d.ID, d.OwnerID, attempts, next, dead, err.Error()); rerr != nil {
			w.log.Warn("embed worker: record failure", "id", d.ID, "err", rerr)
			return false
		}
		w.log.Warn("embed worker: per-doc embed failure", "id", d.ID, "attempts", attempts, "dead", dead, "err", err)
		return true // skip this doc, keep going (no head-of-line block)
	}
	if err := w.docs.ReplaceChunks(ctx, d.ID, d.OwnerID, texts, vecs); err != nil {
		w.log.Warn("embed worker: replace chunks", "id", d.ID, "err", err)
		return false
	}
	return true
}
