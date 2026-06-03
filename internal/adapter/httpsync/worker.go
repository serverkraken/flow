package httpsync

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Worker runs the background sync loop: it pulls from the server every
// pullInterval, drains the local write queue on demand (via SignalPush), and
// emits ports.ConflictMsg on conflict channel when the server returns 409.
//
// # Concurrency contract
//
//   - Start must be called at most once. A sync.Once guard enforces this;
//     a second call is silently ignored.
//   - runPull and runDrain always run sequentially inside the same goroutine
//     (the loop goroutine), so no locking is required on the store writes.
//   - The conflicts channel is sent from the loop goroutine and received by
//     the TUI goroutine; the buffered capacity (16) absorbs bursts. When full,
//     conflicts are logged and dropped rather than blocking the loop.
//   - pushSignal is a buffered channel of capacity 1. Multiple concurrent
//     SignalPush calls coalesce: if a signal is already pending the new one is
//     silently dropped. The loop drains it via a non-blocking send from the
//     caller side.
type Worker struct {
	client     *Client
	sessions   ports.SessionStore
	projects   ports.ProjectStore
	active     ports.ActiveSessionStore
	watermarks ports.SyncWatermarkStore
	queue      *Queue
	userID     string

	conflicts  chan ports.ConflictMsg
	pushSignal chan struct{}
	pullSignal chan struct{} // buffered(1); drained by loop alongside pushSignal
	done       chan struct{} // closed by loop when it exits
	stop       context.CancelFunc

	// PullInterval is the time between automatic pull cycles.
	// Tests may lower this after NewWorker and before Start to avoid 30-second
	// waits; production callers leave it at the default 30s set by NewWorker.
	PullInterval time.Duration

	startOnce sync.Once
}

// NewWorker creates a Worker. The worker does not start until Start is called.
func NewWorker(
	client *Client,
	ss ports.SessionStore,
	ps ports.ProjectStore,
	as ports.ActiveSessionStore,
	ws ports.SyncWatermarkStore,
	q *Queue,
	userID string,
) *Worker {
	return &Worker{
		client:       client,
		sessions:     ss,
		projects:     ps,
		active:       as,
		watermarks:   ws,
		queue:        q,
		userID:       userID,
		conflicts:    make(chan ports.ConflictMsg, 16),
		pushSignal:   make(chan struct{}, 1),
		pullSignal:   make(chan struct{}, 1),
		done:         make(chan struct{}),
		PullInterval: 30 * time.Second,
	}
}

// Conflicts returns the read-only channel on which ports.ConflictMsg values
// are delivered. Task 30/31 consume this channel to render the conflict overlay.
func (w *Worker) Conflicts() <-chan ports.ConflictMsg { return w.conflicts }

// Done returns a channel that is closed when the loop goroutine exits.
// Tests use this to wait for Stop() to take effect without polling goroutine
// counts.
func (w *Worker) Done() <-chan struct{} { return w.done }

// SignalPush triggers an immediate push-drain (called by use-cases after
// Enqueue). Non-blocking — drops the signal if one is already pending; the
// worker will drain on its next pull iteration anyway.
func (w *Worker) SignalPush() {
	select {
	case w.pushSignal <- struct{}{}:
	default:
	}
}

// ForcePull triggers an immediate pull cycle (called by SyncStatus.ForcePull
// which is invoked from the `flow sync force-pull` CLI or the TUI conflict
// overlay). Non-blocking — coalesces with a pending pull signal.
func (w *Worker) ForcePull() {
	select {
	case w.pullSignal <- struct{}{}:
	default:
	}
}

// Start launches the background loop goroutine. It is safe to call from any
// goroutine. If Start has already been called, subsequent calls are no-ops
// (enforced by sync.Once).
func (w *Worker) Start(ctx context.Context) {
	w.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(ctx)
		w.stop = cancel
		go w.loop(ctx)
	})
}

// Stop cancels the loop context, causing the loop goroutine to exit on its
// next iteration. It returns immediately; callers that need to synchronise
// should wait on Done().
func (w *Worker) Stop() {
	if w.stop != nil {
		w.stop()
	}
}

func (w *Worker) loop(ctx context.Context) {
	defer close(w.done)

	pullT := time.NewTicker(w.PullInterval)
	defer pullT.Stop()

	// Initial pull on start so the cache catches up before the first user action.
	w.runPull(ctx)
	w.runDrain(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-pullT.C:
			w.runPull(ctx)
			w.runDrain(ctx)
		case <-w.pushSignal:
			w.runDrain(ctx)
		case <-w.pullSignal:
			w.runPull(ctx)
			w.runDrain(ctx)
		}
	}
}

func (w *Worker) runPull(ctx context.Context) {
	if err := w.pullResource(ctx, "projects"); err != nil {
		slog.Warn("sync: pull projects", slog.Any("err", err))
	}
	if err := w.pullResource(ctx, "sessions"); err != nil {
		slog.Warn("sync: pull sessions", slog.Any("err", err))
	}
	if err := w.pullResource(ctx, "active_sessions"); err != nil {
		slog.Warn("sync: pull active_sessions", slog.Any("err", err))
	}
}

// pullResource handles one resource type: fetch the current watermark, GET the
// resource endpoint, ingest items into the local cache, and advance the
// watermark. Loops while the server signals has_more.
func (w *Worker) pullResource(ctx context.Context, resource string) error {
	for {
		hi, more, err := w.pullResourcePage(ctx, resource)
		if err != nil {
			return err
		}
		if err := w.watermarks.Set(resource, hi); err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
}

// pullResourcePage fetches one page for the given resource, ingests the items,
// and returns (highWatermark, hasMore, err). It reads the current watermark
// from the store to use as the since parameter.
func (w *Worker) pullResourcePage(ctx context.Context, resource string) (hi int64, more bool, err error) {
	wm, _ := w.watermarks.Get(resource)
	switch resource {
	case "sessions":
		return w.pullSessionsPage(ctx, wm)
	case "projects":
		return w.pullProjectsPage(ctx, wm)
	case "active_sessions":
		return w.pullActivePage(ctx, wm)
	default:
		return 0, false, nil
	}
}

func (w *Worker) pullSessionsPage(ctx context.Context, since int64) (int64, bool, error) {
	items, hi, more, err := w.client.PullSessions(ctx, since, 200)
	if err != nil {
		return 0, false, err
	}
	if err := w.sessions.UpsertBatch(items); err != nil {
		return 0, false, err
	}
	return hi, more, nil
}

func (w *Worker) pullProjectsPage(ctx context.Context, since int64) (int64, bool, error) {
	items, hi, more, err := w.client.PullProjects(ctx, since, 200)
	if err != nil {
		return 0, false, err
	}
	for _, p := range items {
		if err := w.projects.Upsert(p); err != nil {
			return 0, false, err
		}
	}
	return hi, more, nil
}

func (w *Worker) pullActivePage(ctx context.Context, since int64) (int64, bool, error) {
	items, hi, err := w.client.PullActive(ctx, since)
	if err != nil {
		return 0, false, err
	}
	for _, a := range items {
		if err := w.active.Upsert(a); err != nil {
			return 0, false, err
		}
	}
	return hi, false, nil // active_sessions has no has_more paging
}

func (w *Worker) runDrain(ctx context.Context) {
	err := w.queue.Drain(func(e ports.WriteQueueEntry) (bool, error) {
		switch e.Resource {
		case "sessions":
			return w.drainSession(ctx, e)
		case "projects":
			return w.drainProject(ctx, e)
		case "active_sessions":
			return w.drainActiveStart(ctx, e)
		case "active_sessions_stop":
			return w.drainActiveStop(ctx, e)
		}
		return false, nil
	})
	if err != nil {
		slog.Warn("sync: drain", slog.Any("err", err))
	}
}

func (w *Worker) drainSession(ctx context.Context, e ports.WriteQueueEntry) (bool, error) {
	var s domain.Session
	if err := json.Unmarshal(e.Payload, &s); err != nil {
		return false, err
	}
	newV, err := w.client.PushSession(ctx, s, e.ExpectedVersion)
	if errors.Is(err, ports.ErrSessionVersionConflict) {
		w.emitConflictFromError(ctx, "sessions", s.ID, e.Seq, s, err)
		return false, nil // halt drain — conflict must be resolved before retrying
	}
	if err != nil {
		return false, err
	}
	s.Version = newV
	_ = w.sessions.Upsert(s) // update local cache with server-confirmed version
	return true, nil
}

func (w *Worker) drainProject(ctx context.Context, e ports.WriteQueueEntry) (bool, error) {
	var p domain.Project
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return false, err
	}
	newV, err := w.client.PushProject(ctx, p, e.ExpectedVersion)
	if errors.Is(err, ports.ErrProjectVersionConflict) {
		w.emitConflictFromError(ctx, "projects", p.ID, e.Seq, p, err)
		return false, nil
	}
	if err != nil {
		return false, err
	}
	p.Version = newV
	_ = w.projects.Upsert(p)
	return true, nil
}

// activeStartBody is the JSON shape written by queue.EnqueueActiveStart.
type activeStartBody struct {
	Action          string `json:"action"`
	ProjectID       string `json:"project_id"`
	StartedOnDevice string `json:"started_on_device"`
	Tag             string `json:"tag"`
	Note            string `json:"note"`
}

func (w *Worker) drainActiveStart(ctx context.Context, e ports.WriteQueueEntry) (bool, error) {
	var body activeStartBody
	if err := json.Unmarshal(e.Payload, &body); err != nil {
		return false, err
	}
	_, err := w.client.StartActive(ctx, e.RowID, body.StartedOnDevice, e.ExpectedVersion, body.Tag, body.Note)
	if errors.Is(err, ports.ErrActiveSessionConflict) {
		w.emitConflictFromError(ctx, "active_sessions", e.RowID, e.Seq, body, err)
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Server returned the canonical active row; the local cache is refreshed on
	// the next pull cycle.
	return true, nil
}

// activeStopBody is the JSON shape written by queue.EnqueueActiveStop.
type activeStopBody struct {
	Action string `json:"action"`
	Tag    string `json:"tag"`
	Note   string `json:"note"`
}

func (w *Worker) drainActiveStop(ctx context.Context, e ports.WriteQueueEntry) (bool, error) {
	var body activeStopBody
	if err := json.Unmarshal(e.Payload, &body); err != nil {
		return false, err
	}
	_, err := w.client.StopActive(ctx, e.RowID, e.ExpectedVersion, body.Tag, body.Note)
	if errors.Is(err, ports.ErrActiveSessionConflict) {
		w.emitConflictFromError(ctx, "active_sessions_stop", e.RowID, e.Seq, body, err)
		return false, nil
	}
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		// Another device already stopped this session — retire the entry as a
		// success since there is nothing left to stop.
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// emitConflictFromError extracts the server's current row from ce (if it is a
// *ConflictError) and sends a ports.ConflictMsg on the conflicts channel.
// Non-blocking: when the channel is full the message is logged and dropped
// rather than stalling the loop goroutine.
func (w *Worker) emitConflictFromError(_ context.Context, resource, rowID string, seq int64, local any, srcErr error) {
	msg := ports.ConflictMsg{
		Resource: resource,
		RowID:    rowID,
		QueueSeq: seq,
		Local:    local,
	}

	var ce *ConflictError
	if errors.As(srcErr, &ce) && len(ce.Current) > 0 {
		switch resource {
		case "sessions":
			var s domain.Session
			if err := json.Unmarshal(ce.Current, &s); err == nil {
				msg.Server = s
			}
		case "projects":
			var p domain.Project
			if err := json.Unmarshal(ce.Current, &p); err == nil {
				msg.Server = p
			}
		case "active_sessions", "active_sessions_stop":
			var a domain.ActiveSession
			if err := json.Unmarshal(ce.Current, &a); err == nil {
				msg.Server = a
			}
		}
	}

	select {
	case w.conflicts <- msg:
	default:
		slog.Warn("sync: conflict channel full, dropping",
			slog.String("resource", resource),
			slog.String("row_id", rowID),
		)
	}
}
