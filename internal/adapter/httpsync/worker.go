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
	repos      ports.RepoStore     // optional — Plan C; nil when caller skips repo/note sync
	notes      ports.RepoNoteStore // optional — Plan C
	watermarks ports.SyncWatermarkStore
	queue      *Queue
	userID     string

	conflicts  chan ports.ConflictMsg
	pushSignal chan struct{}
	pullSignal chan struct{} // buffered(1); drained by loop alongside pushSignal
	pullDone   chan struct{} // buffered(1); coalesced signal fired after each successful runPull
	done       chan struct{} // closed by loop when it exits
	stop       context.CancelFunc

	// PullInterval is the time between automatic pull cycles.
	// Tests may lower this after NewWorker and before Start to avoid 30-second
	// waits; production callers leave it at the default 30s set by NewWorker.
	PullInterval time.Duration

	// Backoff schedules retries for transient drain failures. Zero value
	// resolves to the defaults documented on Backoff (500ms/60s/2x/±20%).
	// Tests override with a tight {Base: ms, Factor: 2} so retries land
	// quickly.
	Backoff Backoff

	startOnce sync.Once
}

// NewWorker creates a Worker. The worker does not start until Start is called.
//
// Plan-C resource fields (repos, notes) are set via SetRepoStores after
// construction rather than added to NewWorker's signature — keeps the
// existing call sites green and signals that those stores are optional
// (the worker silently skips repo/note pull+drain when nil).
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
		pullDone:     make(chan struct{}, 1),
		done:         make(chan struct{}),
		PullInterval: 30 * time.Second,
	}
}

// SetRepoStores wires the Plan-C resource stores onto the worker. Both
// args must be non-nil for the repo/note sync paths to be active; passing
// nil keeps the original (sessions+projects+active) behaviour.
func (w *Worker) SetRepoStores(repos ports.RepoStore, notes ports.RepoNoteStore) {
	w.repos = repos
	w.notes = notes
}

// Conflicts returns the read-only channel on which ports.ConflictMsg values
// are delivered. Task 30/31 consume this channel to render the conflict overlay.
func (w *Worker) Conflicts() <-chan ports.ConflictMsg { return w.conflicts }

// PullDone returns a read-only channel that receives a signal after each
// successful runPull completes (i.e. no ErrUnauthorized short-circuit). The
// channel has capacity 1 and uses a non-blocking send, so rapid successive
// pulls coalesce: listeners receive at most one notification per drain of the
// channel. The signal carries no data — a unit struct suffices.
//
// The TUI wires this via a bubbletea Cmd that blocks until a signal arrives and
// then emits a ChangedMsg, causing all worktime sub-tabs to reload immediately
// when cross-device data lands — instead of waiting up to 10 s for the next
// tick.
func (w *Worker) PullDone() <-chan struct{} { return w.pullDone }

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
	resources := []string{"projects", "sessions", "active_sessions"}
	if w.repos != nil {
		resources = append(resources, "repos")
	}
	if w.notes != nil {
		resources = append(resources, "repo_notes")
	}
	for _, res := range resources {
		if err := w.pullResource(ctx, res); err != nil {
			if errors.Is(err, ErrUnauthorized) {
				// Logged-out is the normal offline state — one debug line,
				// skip the remaining resources (they fail identically).
				slog.Debug("sync: pull skipped — not logged in")
				return
			}
			slog.Warn("sync: pull "+res, slog.Any("err", err))
		}
	}
	// Signal that the pull cycle completed so TUI listeners can reload
	// without waiting for the next tick. Non-blocking: if the buffer is
	// already full (listener hasn't drained yet) the signal coalesces.
	select {
	case w.pullDone <- struct{}{}:
	default:
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
	case "repos":
		return w.pullReposPage(ctx, wm)
	case "repo_notes":
		return w.pullRepoNotesPage(ctx, wm)
	default:
		return 0, false, nil
	}
}

func (w *Worker) pullReposPage(ctx context.Context, since int64) (int64, bool, error) {
	items, hi, more, err := w.client.PullRepos(ctx, since, 200)
	if err != nil {
		return 0, false, err
	}
	// The server scopes each pull to the authenticated user, so every pulled row
	// belongs to the local user; rewriting UserID keeps the local FK (user_id →
	// users(id)) satisfiable when the server's user UUID differs from the client's.
	for _, r := range items {
		r.UserID = w.userID
		if err := w.repos.Upsert(r); err != nil {
			return 0, false, err
		}
	}
	return hi, more, nil
}

func (w *Worker) pullRepoNotesPage(ctx context.Context, since int64) (int64, bool, error) {
	items, hi, more, err := w.client.PullRepoNotes(ctx, since, 200)
	if err != nil {
		return 0, false, err
	}
	// The server scopes each pull to the authenticated user, so every pulled row
	// belongs to the local user; rewriting UserID keeps the local FK (user_id →
	// users(id)) satisfiable when the server's user UUID differs from the client's.
	for _, n := range items {
		n.UserID = w.userID
		if err := w.notes.Upsert(n); err != nil {
			return 0, false, err
		}
	}
	return hi, more, nil
}

func (w *Worker) pullSessionsPage(ctx context.Context, since int64) (int64, bool, error) {
	items, hi, more, err := w.client.PullSessions(ctx, since, 200)
	if err != nil {
		return 0, false, err
	}
	// The server scopes each pull to the authenticated user, so every pulled row
	// belongs to the local user; rewriting UserID keeps the local FK (user_id →
	// users(id)) satisfiable when the server's user UUID differs from the client's.
	for i := range items {
		items[i].UserID = w.userID
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
	// The server scopes each pull to the authenticated user, so every pulled row
	// belongs to the local user; rewriting UserID keeps the local FK (user_id →
	// users(id)) satisfiable when the server's user UUID differs from the client's.
	for _, p := range items {
		p.UserID = w.userID
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
	// The server scopes each pull to the authenticated user, so every pulled row
	// belongs to the local user; rewriting UserID keeps the local FK (user_id →
	// users(id)) satisfiable when the server's user UUID differs from the client's.
	for _, a := range items {
		a.UserID = w.userID
		if err := w.active.Upsert(a); err != nil {
			return 0, false, err
		}
	}
	return hi, false, nil // active_sessions has no has_more paging
}

func (w *Worker) runDrain(ctx context.Context) {
	err := w.queue.Drain(func(e ports.WriteQueueEntry) (DrainAction, error) {
		switch e.Resource {
		case "sessions":
			return w.drainSession(ctx, e)
		case "projects":
			return w.drainProject(ctx, e)
		case "active_sessions":
			return w.drainActiveStart(ctx, e)
		case "active_sessions_stop":
			return w.drainActiveStop(ctx, e)
		case "repos":
			return w.drainRepo(ctx, e)
		case "repo_notes":
			return w.drainRepoNote(ctx, e)
		}
		// Unknown resource — drop it as a permanent failure so it doesn't
		// block the queue forever.
		slog.Warn("sync: drain unknown resource, dropping", slog.String("resource", e.Resource), slog.Int64("seq", e.Seq))
		return DrainAck, nil
	}, w.Backoff)
	if err != nil {
		slog.Warn("sync: drain", slog.Any("err", err))
	}
}

// classifyPushError maps a push error to a (DrainAction, error) pair.
//
//   - JSON unmarshal errors are permanent (data corruption — retrying won't help).
//   - *PermanentError (4xx other than 401/404/408/409/429) → DrainAck + log.
//   - Anything else (transport errors, 5xx, 408/429, ErrUnauthorized) → DrainRetry.
//
// Conflict (409) and active-session-stop 404 are handled before this point
// in each drain* method.
func (w *Worker) classifyPushError(resource, rowID string, seq int64, err error) (DrainAction, error) {
	var perm *PermanentError
	if errors.As(err, &perm) {
		slog.Warn(
			"sync: permanent push failure — dropping entry",
			slog.String("resource", resource),
			slog.String("row_id", rowID),
			slog.Int64("seq", seq),
			slog.Int("status", perm.Status),
			slog.String("body", perm.Body),
		)
		return DrainAck, err
	}
	level := slog.LevelInfo
	if errors.Is(err, ErrUnauthorized) {
		level = slog.LevelDebug // logged-out retries are routine, not noteworthy
	}
	slog.Log(
		context.Background(), level,
		"sync: transient push failure — scheduling retry",
		slog.String("resource", resource),
		slog.String("row_id", rowID),
		slog.Int64("seq", seq),
		slog.Any("err", err),
	)
	return DrainRetry, err
}

func (w *Worker) drainSession(ctx context.Context, e ports.WriteQueueEntry) (DrainAction, error) {
	var s domain.Session
	if err := json.Unmarshal(e.Payload, &s); err != nil {
		// Corrupted payload — Ack so the queue isn't blocked forever.
		return DrainAck, err
	}
	newV, err := w.client.PushSession(ctx, s, e.ExpectedVersion)
	if errors.Is(err, ports.ErrSessionVersionConflict) {
		w.emitConflictFromError(ctx, "sessions", s.ID, e.Seq, s, err)
		return DrainHalt, nil // conflict must be resolved before retrying
	}
	if err != nil {
		return w.classifyPushError("sessions", s.ID, e.Seq, err)
	}
	s.Version = newV
	_ = w.sessions.Upsert(s) // update local cache with server-confirmed version
	return DrainAck, nil
}

func (w *Worker) drainProject(ctx context.Context, e ports.WriteQueueEntry) (DrainAction, error) {
	var p domain.Project
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return DrainAck, err
	}
	newV, err := w.client.PushProject(ctx, p, e.ExpectedVersion)
	if errors.Is(err, ports.ErrProjectVersionConflict) {
		w.emitConflictFromError(ctx, "projects", p.ID, e.Seq, p, err)
		return DrainHalt, nil
	}
	if err != nil {
		return w.classifyPushError("projects", p.ID, e.Seq, err)
	}
	p.Version = newV
	_ = w.projects.Upsert(p)
	return DrainAck, nil
}

func (w *Worker) drainRepo(ctx context.Context, e ports.WriteQueueEntry) (DrainAction, error) {
	var r domain.Repo
	if err := json.Unmarshal(e.Payload, &r); err != nil {
		return DrainAck, err
	}
	newV, err := w.client.PushRepo(ctx, r, e.ExpectedVersion)
	if errors.Is(err, ports.ErrRepoVersionConflict) {
		w.emitConflictFromError(ctx, "repos", r.ID, e.Seq, r, err)
		return DrainHalt, nil
	}
	if err != nil {
		return w.classifyPushError("repos", r.ID, e.Seq, err)
	}
	r.Version = newV
	if w.repos != nil {
		_ = w.repos.Upsert(r)
	}
	return DrainAck, nil
}

func (w *Worker) drainRepoNote(ctx context.Context, e ports.WriteQueueEntry) (DrainAction, error) {
	var n domain.RepoNote
	if err := json.Unmarshal(e.Payload, &n); err != nil {
		return DrainAck, err
	}
	newV, err := w.client.PushRepoNote(ctx, n, e.ExpectedVersion)
	if errors.Is(err, ports.ErrRepoNoteVersionConflict) {
		w.emitConflictFromError(ctx, "repo_notes", n.ID, e.Seq, n, err)
		return DrainHalt, nil
	}
	if err != nil {
		return w.classifyPushError("repo_notes", n.ID, e.Seq, err)
	}
	n.Version = newV
	if w.notes != nil {
		_ = w.notes.Upsert(n)
	}
	return DrainAck, nil
}

// activeStartBody is the JSON shape written by usecase.encodeActiveStart.
type activeStartBody struct {
	Action          string    `json:"action"`
	ProjectID       string    `json:"project_id"`
	StartedAt       time.Time `json:"started_at"`
	StartedOnDevice string    `json:"started_on_device"`
	Tag             string    `json:"tag"`
	Note            string    `json:"note"`
}

func (w *Worker) drainActiveStart(ctx context.Context, e ports.WriteQueueEntry) (DrainAction, error) {
	var body activeStartBody
	if err := json.Unmarshal(e.Payload, &body); err != nil {
		return DrainAck, err
	}
	srv, err := w.client.StartActive(ctx, e.RowID, body.StartedAt, body.StartedOnDevice, e.ExpectedVersion, body.Tag, body.Note)
	if errors.Is(err, ports.ErrActiveSessionConflict) {
		w.emitConflictFromError(ctx, "active_sessions", e.RowID, e.Seq, body, err)
		return DrainHalt, nil
	}
	if err != nil {
		return w.classifyPushError("active_sessions", e.RowID, e.Seq, err)
	}
	// Write the server-assigned version back to the local row so the later
	// Stop's If-Match matches. Skip when the row is already gone (user
	// stopped while offline — the queued stop reconciles via the 409-retry
	// in drainActiveStop); upserting here would resurrect a finished session.
	if _, gerr := w.active.Get(w.userID, e.RowID); gerr == nil {
		// gerr != nil covers ErrActiveSessionNotFound (user stopped offline —
		// skip resurrection) and transient I/O errors (safe to skip; the next
		// drain attempt will retry).
		srv.UserID = w.userID
		if uerr := w.active.Upsert(srv); uerr != nil {
			slog.Warn("sync: active-start write-back failed; next stop may 409",
				slog.String("project_id", e.RowID),
				slog.Any("err", uerr))
		}
	}
	return DrainAck, nil
}

// activeStopBody is the JSON shape for an active-session stop payload.
// Both the encoder in internal/usecase/active_sessions.go (ActiveSessions.Stop)
// and this decoder MUST use the same JSON field names; neither side may add
// or rename a field without updating the other.
type activeStopBody struct {
	Action string `json:"action"`
	Tag    string `json:"tag"`
	Note   string `json:"note"`
}

func (w *Worker) drainActiveStop(ctx context.Context, e ports.WriteQueueEntry) (DrainAction, error) {
	var body activeStopBody
	if err := json.Unmarshal(e.Payload, &body); err != nil {
		return DrainAck, err
	}
	_, err := w.client.StopActive(ctx, e.RowID, e.ExpectedVersion, body.Tag, body.Note)
	if errors.Is(err, ports.ErrActiveSessionConflict) {
		// Stale local version (start drained after the stop was enqueued).
		// The 409 body carries the server's current row — retry once with
		// that version. Stopping our own session is last-writer-wins for
		// the single-user PoC; a failing retry still halts for the overlay.
		// Two RPCs in one drain step deliberately — avoids a re-queue cycle; acceptable for the single-user PoC.
		if cur, ok := conflictCurrentActive(err); ok {
			_, rerr := w.client.StopActive(ctx, e.RowID, cur.Version, body.Tag, body.Note)
			if rerr == nil || errors.Is(rerr, ports.ErrActiveSessionNotFound) {
				slog.Debug("sync: active-stop 409 retry succeeded",
					slog.String("project_id", e.RowID),
					slog.Int64("version", cur.Version))
				return DrainAck, nil
			}
		}
		w.emitConflictFromError(ctx, "active_sessions_stop", e.RowID, e.Seq, body, err)
		return DrainHalt, nil
	}
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		// Another device already stopped this session — retire the entry as a
		// success since there is nothing left to stop.
		return DrainAck, nil
	}
	if err != nil {
		return w.classifyPushError("active_sessions_stop", e.RowID, e.Seq, err)
	}
	return DrainAck, nil
}

// conflictCurrentActive extracts the server's current ActiveSession from a
// 409 ConflictError, when present.
func conflictCurrentActive(err error) (domain.ActiveSession, bool) {
	var ce *ConflictError
	if !errors.As(err, &ce) || len(ce.Current) == 0 {
		return domain.ActiveSession{}, false
	}
	var cur domain.ActiveSession
	if jerr := json.Unmarshal(ce.Current, &cur); jerr != nil {
		return domain.ActiveSession{}, false
	}
	return cur, true
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
		slog.Warn(
			"sync: conflict channel full, dropping",
			slog.String("resource", resource),
			slog.String("row_id", rowID),
		)
	}
}
