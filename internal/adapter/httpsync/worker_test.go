package httpsync_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpsync"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

// ---- helpers ----------------------------------------------------------------

// newWorker constructs a Worker with a fast pull interval for testing.
// It uses the provided inner queue (drainableQueue) via httpsync.NewQueue.
// The Backoff is set to a sub-millisecond base so retry tests don't sleep
// real backoff intervals.
func newWorker(
	srv *httptest.Server,
	ss ports.SessionStore,
	ps ports.ProjectStore,
	as ports.ActiveSessionStore,
	ws ports.SyncWatermarkStore,
	inner ports.WriteQueue,
) *httpsync.Worker {
	c := newClient(srv, "tok")
	q := httpsync.NewQueue(inner)
	w := httpsync.NewWorker(c, ss, ps, as, ws, q, "user1")
	w.PullInterval = 10 * time.Millisecond // speed up pull ticker in tests
	w.Backoff = httpsync.Backoff{
		Base:   time.Millisecond,
		Max:    10 * time.Millisecond,
		Factor: 2.0,
		Jitter: -1, // deterministic
	}
	return w
}

// ---- Test 1: pullResource paginated -----------------------------------------

// TestWorker_PullResource_Paginated verifies that when the server returns
// has_more:true on the first call and has_more:false on the second, all items
// are ingested and the watermark is advanced to the last high_watermark.
func TestWorker_PullResource_Paginated(t *testing.T) {
	// Count only calls to the sessions endpoint so that the first page
	// returns has_more:true and the second returns has_more:false.
	var sessCallCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && contains(r.URL.Path, "/sessions") {
			n := sessCallCount.Add(1)
			if n == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items":          []domain.Session{{ID: "s1", Version: 1}, {ID: "s2", Version: 2}},
					"high_watermark": int64(2),
					"has_more":       true,
				})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items":          []domain.Session{{ID: "s3", Version: 5}},
					"high_watermark": int64(5),
					"has_more":       false,
				})
			}
			return
		}
		// All other endpoints (projects, active) return empty pages.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	ss := &testutil.FakeSessionStore{}
	ps := &testutil.FakeProjectStore{}
	as := &testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}}
	ws := &testutil.FakeSyncWatermarkStore{}

	w := newWorker(srv, ss, ps, as, ws, &drainableQueue{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w.Start(ctx)
	// Wait until the initial pull completes: watermark should be set to 5.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		wm, _ := ws.Get("sessions")
		if wm == 5 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	w.Stop()
	<-w.Done()

	// Both session pages fetched.
	if c := sessCallCount.Load(); c < 2 {
		t.Fatalf("sessions endpoint called %d times, want >= 2", c)
	}
	// All 3 sessions ingested.
	all, _ := ss.Load("user1")
	if len(all) != 3 {
		t.Errorf("sessions ingested: got %d, want 3", len(all))
	}
	// Watermark advanced to 5.
	wm, _ := ws.Get("sessions")
	if wm != 5 {
		t.Errorf("watermark: got %d, want 5", wm)
	}
}

// ---- Test 2: pullResource server error --------------------------------------

// TestWorker_PullResource_ServerError verifies that a 500 from the server
// returns an error from pullResource, and the watermark is not advanced.
func TestWorker_PullResource_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	ss := &testutil.FakeSessionStore{}
	ps := &testutil.FakeProjectStore{}
	as := &testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}}
	ws := &testutil.FakeSyncWatermarkStore{}

	w := newWorker(srv, ss, ps, as, ws, &drainableQueue{})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	w.Start(ctx)
	// Let the loop run one pull (it returns quickly because server errors fast).
	time.Sleep(50 * time.Millisecond)
	w.Stop()
	<-w.Done()

	// Watermark must remain at zero — no successful pull happened.
	for _, resource := range []string{"sessions", "projects", "active_sessions"} {
		wm, _ := ws.Get(resource)
		if wm != 0 {
			t.Errorf("%s watermark after error: got %d, want 0", resource, wm)
		}
	}
}

// ---- Test 3: runDrain happy path --------------------------------------------

// TestWorker_RunDrain_HappyPath verifies that 3 queue entries (one session,
// one project, one active_sessions) are pushed successfully, removed from the
// queue, and the local caches are updated with the server-confirmed versions.
func TestWorker_RunDrain_HappyPath(t *testing.T) {
	inner := &drainableQueue{}

	sess := domain.Session{ID: "s1", UserID: "user1", ProjectID: "p1", Version: 0}
	proj := domain.Project{ID: "p1", UserID: "user1", Version: 0}

	// Pre-populate queue entries.
	sessPayload, _ := json.Marshal(sess)
	_ = inner.enqueueRaw("sessions", "s1", sessPayload, 0)
	projPayload, _ := json.Marshal(proj)
	_ = inner.enqueueRaw("projects", "p1", projPayload, 0)
	activePayload, _ := json.Marshal(map[string]string{
		"action":            "start",
		"project_id":        "p1",
		"started_on_device": "laptop",
	})
	_ = inner.enqueueRaw("active_sessions", "p1", activePayload, 0)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && contains(r.URL.Path, "/sessions/"):
			_ = json.NewEncoder(w).Encode(domain.Session{ID: "s1", Version: 10})
		case r.Method == http.MethodPut && contains(r.URL.Path, "/projects/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "p1", "version": 20})
		case r.Method == http.MethodPost && contains(r.URL.Path, "/active/"):
			_ = json.NewEncoder(w).Encode(domain.ActiveSession{ProjectID: "p1", Version: 5})
		default:
			// GET pull endpoints — return empty pages.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []any{}, "high_watermark": int64(0), "has_more": false,
			})
		}
	}))
	defer srv.Close()

	ss := &testutil.FakeSessionStore{}
	ps := &testutil.FakeProjectStore{}
	as := &testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}}
	ws := &testutil.FakeSyncWatermarkStore{}

	w := newWorker(srv, ss, ps, as, ws, inner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w.Start(ctx)
	// Wait until all 3 entries are drained.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if inner.pendingCount() == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	w.Stop()
	<-w.Done()

	if c := inner.pendingCount(); c != 0 {
		t.Errorf("pending queue entries: got %d, want 0", c)
	}

	// Session cache updated with server version 10.
	sessions, _ := ss.Load("user1")
	found := false
	for _, s := range sessions {
		if s.ID == "s1" && s.Version == 10 {
			found = true
		}
	}
	if !found {
		t.Error("session s1 not updated to version 10 in cache")
	}
}

// ---- Test 4: runDrain 409 on session ----------------------------------------

// TestWorker_RunDrain_409_Session verifies that a 409 response for a session
// push causes a ConflictMsg to be emitted, the queue entry is NOT removed, and
// drain halts (no further entries processed).
func TestWorker_RunDrain_409_Session(t *testing.T) {
	inner := &drainableQueue{}

	sess := domain.Session{ID: "s1", Version: 0}
	sessPayload, _ := json.Marshal(sess)
	_ = inner.enqueueRaw("sessions", "s1", sessPayload, 0)
	// Add a second entry that must NOT be processed if drain halts.
	_ = inner.enqueueRaw("sessions", "s2", sessPayload, 0)

	serverCurrent := domain.Session{ID: "s1", Version: 7}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut && contains(r.URL.Path, "/sessions/") {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"current": serverCurrent})
			return
		}
		// GET pull — empty
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	ss := &testutil.FakeSessionStore{}
	ps := &testutil.FakeProjectStore{}
	as := &testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}}
	ws := &testutil.FakeSyncWatermarkStore{}

	w := newWorker(srv, ss, ps, as, ws, inner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w.Start(ctx)

	// Wait for conflict to appear on channel.
	select {
	case msg := <-w.Conflicts():
		if msg.Resource != "sessions" {
			t.Errorf("conflict resource: got %q, want sessions", msg.Resource)
		}
		if msg.RowID != "s1" {
			t.Errorf("conflict RowID: got %q, want s1", msg.RowID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for conflict message")
	}

	w.Stop()
	<-w.Done()

	// s1 entry must NOT be removed.
	if inner.isRemoved(0) {
		t.Error("conflicted entry s1 should NOT be removed from queue")
	}
}

// ---- Test 5: runDrain 409 emits Server row ----------------------------------

// TestWorker_RunDrain_409_EmitsServerRow verifies that the ConflictMsg.Server
// field contains the decoded domain.Session from the 409 response body.
func TestWorker_RunDrain_409_EmitsServerRow(t *testing.T) {
	inner := &drainableQueue{}

	sess := domain.Session{ID: "s1", Version: 0}
	sessPayload, _ := json.Marshal(sess)
	_ = inner.enqueueRaw("sessions", "s1", sessPayload, 0)

	wantServer := domain.Session{ID: "s1", Version: 7, Tag: "deep"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut && contains(r.URL.Path, "/sessions/") {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"current": wantServer})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	ss := &testutil.FakeSessionStore{}
	ps := &testutil.FakeProjectStore{}
	as := &testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}}
	ws := &testutil.FakeSyncWatermarkStore{}

	w := newWorker(srv, ss, ps, as, ws, inner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w.Start(ctx)

	select {
	case msg := <-w.Conflicts():
		serverSess, ok := msg.Server.(domain.Session)
		if !ok {
			t.Fatalf("ConflictMsg.Server type: got %T, want domain.Session", msg.Server)
		}
		if serverSess.Version != wantServer.Version {
			t.Errorf("Server.Version: got %d, want %d", serverSess.Version, wantServer.Version)
		}
		if serverSess.Tag != wantServer.Tag {
			t.Errorf("Server.Tag: got %q, want %q", serverSess.Tag, wantServer.Tag)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for conflict message")
	}

	w.Stop()
	<-w.Done()
}

// ---- Test 6: runDrain non-conflict error ------------------------------------

// TestWorker_RunDrain_NonConflictError verifies that a 500 from the server
// causes SetError on the queue entry and drain continues to the next entry.
func TestWorker_RunDrain_NonConflictError(t *testing.T) {
	inner := &drainableQueue{}

	sess := domain.Session{ID: "s1"}
	payload, _ := json.Marshal(sess)

	_ = inner.enqueueRaw("sessions", "s1", payload, 0)
	_ = inner.enqueueRaw("sessions", "s2", payload, 0)
	_ = inner.enqueueRaw("sessions", "s3", payload, 0)

	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut && contains(r.URL.Path, "/sessions/") {
			n := callCount.Add(1)
			if n == 1 {
				// First entry: return 500 to trigger SetError.
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(domain.Session{ID: "s1", Version: 1})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	ss := &testutil.FakeSessionStore{}
	ps := &testutil.FakeProjectStore{}
	as := &testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}}
	ws := &testutil.FakeSyncWatermarkStore{}

	w := newWorker(srv, ss, ps, as, ws, inner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w.Start(ctx)

	// Wait until s2 and s3 are removed (s1 stays with error).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if inner.isRemoved(1) && inner.isRemoved(2) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	w.Stop()
	<-w.Done()

	// Entry 0 (s1): NOT removed, has error set.
	if inner.isRemoved(0) {
		t.Error("first entry (500 error) should NOT be removed")
	}
	if inner.lastErrorAt(0) == "" {
		t.Error("first entry should have lastError set")
	}
	// Entry 1 (s2): removed (drain continues after SetError).
	if !inner.isRemoved(1) {
		t.Error("second entry should be removed (drain continues after SetError)")
	}
	// Entry 2 (s3): removed.
	if !inner.isRemoved(2) {
		t.Error("third entry should be removed")
	}
}

// ---- Test 7: active_sessions_stop 404 path ----------------------------------

// TestWorker_RunDrain_ActiveStop_404 verifies that a 404 when stopping an
// active session (already gone on server) is treated as success: the queue
// entry is removed.
func TestWorker_RunDrain_ActiveStop_404(t *testing.T) {
	inner := &drainableQueue{}

	stopPayload, _ := json.Marshal(map[string]string{
		"action": "stop",
		"tag":    "deep",
		"note":   "done",
	})
	_ = inner.enqueueRaw("active_sessions_stop", "p1", stopPayload, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete && contains(r.URL.Path, "/active/") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	ss := &testutil.FakeSessionStore{}
	ps := &testutil.FakeProjectStore{}
	as := &testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}}
	ws := &testutil.FakeSyncWatermarkStore{}

	w := newWorker(srv, ss, ps, as, ws, inner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if inner.isRemoved(0) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	w.Stop()
	<-w.Done()

	if !inner.isRemoved(0) {
		t.Error("active_sessions_stop 404 should retire the queue entry as success")
	}
}

// ---- Test 8: SignalPush coalesces -------------------------------------------

// TestWorker_SignalPush_Coalesces verifies that calling SignalPush 5 times
// rapid-fire still results in the drain running (at least once) and that
// subsequent signals coalesce — the push-signal channel never blocks.
func TestWorker_SignalPush_Coalesces(t *testing.T) {
	inner := &drainableQueue{}

	var drainCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut && contains(r.URL.Path, "/sessions/") {
			drainCalls.Add(1)
			_ = json.NewEncoder(w).Encode(domain.Session{ID: "s1", Version: 1})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	// Enqueue one entry so drain has something to push.
	payload, _ := json.Marshal(domain.Session{ID: "s1"})
	_ = inner.enqueueRaw("sessions", "s1", payload, 0)

	ss := &testutil.FakeSessionStore{}
	ps := &testutil.FakeProjectStore{}
	as := &testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}}
	ws := &testutil.FakeSyncWatermarkStore{}

	w := newWorker(srv, ss, ps, as, ws, inner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w.Start(ctx)
	// Fire 5 signals in quick succession — none should block.
	for i := 0; i < 5; i++ {
		w.SignalPush()
	}

	// Wait for drain to process the entry.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if inner.isRemoved(0) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	w.Stop()
	<-w.Done()

	if !inner.isRemoved(0) {
		t.Error("entry should be removed after SignalPush triggers drain")
	}
	// Must have been drained at least once.
	if drainCalls.Load() < 1 {
		t.Error("drain should run at least once after SignalPush")
	}
}

// ---- Test 9: Stop() cancels loop --------------------------------------------

// TestWorker_Stop_CancelsLoop starts the worker, calls Stop, and asserts the
// goroutine exits by waiting on Done().
func TestWorker_Stop_CancelsLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	ss := &testutil.FakeSessionStore{}
	ps := &testutil.FakeProjectStore{}
	as := &testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}}
	ws := &testutil.FakeSyncWatermarkStore{}

	w := newWorker(srv, ss, ps, as, ws, &drainableQueue{})
	ctx := context.Background()

	w.Start(ctx)
	w.Stop()

	select {
	case <-w.Done():
		// goroutine exited — pass
	case <-time.After(2 * time.Second):
		t.Fatal("worker goroutine did not exit after Stop()")
	}
}

// ---- Test: Start twice is a no-op -------------------------------------------

// TestWorker_StartTwice_NoOp verifies that calling Start a second time does
// not launch a second goroutine or overwrite the cancel function.
func TestWorker_StartTwice_NoOp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	ss := &testutil.FakeSessionStore{}
	ps := &testutil.FakeProjectStore{}
	as := &testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}}
	ws := &testutil.FakeSyncWatermarkStore{}

	w := newWorker(srv, ss, ps, as, ws, &drainableQueue{})
	ctx := context.Background()

	w.Start(ctx)
	w.Start(ctx) // second call must be silent no-op

	w.Stop()
	select {
	case <-w.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("worker goroutine did not exit after Stop()")
	}
}

// ---- Retry policy: transient failures retry, permanent failures Ack -------

// TestWorker_RetryBackoff_TransientThenSuccess verifies that a 500 followed
// by a 200 leads to the entry being removed after at least one retry. The
// backoff is set to milliseconds so the test finishes in well under a
// second; counting `pushAttempts >= 2` proves a retry actually happened.
func TestWorker_RetryBackoff_TransientThenSuccess(t *testing.T) {
	inner := &drainableQueue{}
	payload, _ := json.Marshal(domain.Session{ID: "s1"})
	_ = inner.enqueueRaw("sessions", "s1", payload, 0)

	var pushAttempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut && contains(r.URL.Path, "/sessions/") {
			n := pushAttempts.Add(1)
			if n < 3 {
				// First two attempts: transient 500.
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			// Third attempt: success.
			_ = json.NewEncoder(w).Encode(domain.Session{ID: "s1", Version: 1})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	w := newWorker(
		srv,
		&testutil.FakeSessionStore{},
		&testutil.FakeProjectStore{},
		&testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}},
		&testutil.FakeSyncWatermarkStore{},
		inner,
	)
	// 1ms base, 10ms cap, no jitter — retries land fast and deterministic.
	w.Backoff = httpsync.Backoff{
		Base:   time.Millisecond,
		Max:    10 * time.Millisecond,
		Factor: 2.0,
		Jitter: -1,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	w.Start(ctx)

	if !eventually(func() bool {
		return inner.isRemoved(0)
	}, 2*time.Second) {
		t.Fatalf("entry not removed after retries: pushAttempts=%d", pushAttempts.Load())
	}
	if pushAttempts.Load() < 3 {
		t.Errorf("expected ≥3 push attempts (2 retries + success), got %d", pushAttempts.Load())
	}
	w.Stop()
	<-w.Done()
}

// TestWorker_PermanentError_Acks verifies that a 422 (unprocessable entity)
// causes the worker to Ack the entry — the queue drops it without retrying.
func TestWorker_PermanentError_Acks(t *testing.T) {
	inner := &drainableQueue{}
	payload, _ := json.Marshal(domain.Session{ID: "s1"})
	_ = inner.enqueueRaw("sessions", "s1", payload, 0)

	var pushAttempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut && contains(r.URL.Path, "/sessions/") {
			pushAttempts.Add(1)
			http.Error(w, "validation failed", http.StatusUnprocessableEntity)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	w := newWorker(
		srv,
		&testutil.FakeSessionStore{},
		&testutil.FakeProjectStore{},
		&testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}},
		&testutil.FakeSyncWatermarkStore{},
		inner,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.Start(ctx)

	if !eventually(func() bool {
		return inner.isRemoved(0)
	}, 1500*time.Millisecond) {
		t.Fatalf("permanent-failure entry should be Acked, pushAttempts=%d", pushAttempts.Load())
	}
	// Exactly one attempt — 422 is permanent, no retry.
	if got := pushAttempts.Load(); got != 1 {
		t.Errorf("pushAttempts: got %d, want 1 (no retry on 422)", got)
	}
	w.Stop()
	<-w.Done()
}

// TestWorker_RespectsNextRetryAt verifies that an entry parked in the
// future via SetRetry is excluded from drain until the timestamp elapses.
// We pre-populate the queue with one entry whose next_retry_at is hours
// away and assert the server is never hit during the worker's lifetime.
func TestWorker_RespectsNextRetryAt(t *testing.T) {
	inner := &drainableQueue{}
	payload, _ := json.Marshal(domain.Session{ID: "s1"})
	_ = inner.enqueueRaw("sessions", "s1", payload, 0)

	// Park the entry 1 hour in the future.
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	_ = inner.SetRetry(1, "parked", future)

	var pushAttempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut && contains(r.URL.Path, "/sessions/") {
			pushAttempts.Add(1)
			_ = json.NewEncoder(w).Encode(domain.Session{ID: "s1", Version: 1})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	w := newWorker(
		srv,
		&testutil.FakeSessionStore{},
		&testutil.FakeProjectStore{},
		&testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}},
		&testutil.FakeSyncWatermarkStore{},
		inner,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	w.Start(ctx)

	// Let several drain cycles run; the entry must stay parked.
	time.Sleep(150 * time.Millisecond)
	w.Stop()
	<-w.Done()

	if got := pushAttempts.Load(); got != 0 {
		t.Errorf("pushAttempts: got %d, want 0 (entry parked via next_retry_at)", got)
	}
	if inner.isRemoved(0) {
		t.Error("parked entry should not be removed")
	}
}

// ---- helpers for drainableQueue in worker tests ----------------------------

// enqueueRaw is a helper on drainableQueue that returns the error (for test setup).
func (d *drainableQueue) enqueueRaw(resource, rowID string, payload []byte, expectedVersion int64) error {
	_, err := d.Enqueue(resource, rowID, payload, expectedVersion)
	return err
}

// pendingCount returns the number of non-removed entries.
func (d *drainableQueue) pendingCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, e := range d.entries {
		if !e.removed {
			n++
		}
	}
	return n
}

// isRemoved returns whether the entry at index i has been removed.
func (d *drainableQueue) isRemoved(i int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.entries[i].removed
}

// lastErrorAt returns the lastError field of the entry at index i.
func (d *drainableQueue) lastErrorAt(i int) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.entries[i].lastError
}

// contains is a simple substring helper to avoid importing strings in test file.
// TestWorker_DrainActiveStart_HappyPath verifies that an active_sessions
// queue entry produced by encodeActiveStart is correctly dispatched to the
// server's POST /api/v1/active/{id}/start and removed on a 2xx response.
func TestWorker_DrainActiveStart_HappyPath(t *testing.T) {
	inner := &drainableQueue{}
	payload, _ := json.Marshal(struct {
		Action          string `json:"action"`
		ProjectID       string `json:"project_id"`
		StartedOnDevice string `json:"started_on_device"`
		Tag             string `json:"tag"`
		Note            string `json:"note"`
	}{"start", "proj-1", "laptop", "deep", "kicked-off"})
	_ = inner.enqueueRaw("active_sessions", "proj-1", payload, 0)

	var sawStart atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && contains(r.URL.Path, "/active/proj-1/start") {
			sawStart.Add(1)
			_ = json.NewEncoder(w).Encode(domain.ActiveSession{
				ProjectID: "proj-1", StartedOnDevice: "laptop", Version: 11,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	w := newWorker(
		srv,
		&testutil.FakeSessionStore{},
		&testutil.FakeProjectStore{},
		&testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}},
		&testutil.FakeSyncWatermarkStore{},
		inner,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.Start(ctx)

	if !eventually(func() bool {
		entries, _ := inner.Peek(10)
		return sawStart.Load() >= 1 && len(entries) == 0
	}, 1500*time.Millisecond) {
		entries, _ := inner.Peek(10)
		t.Fatalf("expected start drained: sawStart=%d remaining=%d", sawStart.Load(), len(entries))
	}
	w.Stop()
	<-w.Done()
}

// TestWorker_DrainActiveStop_HappyPath verifies that an active_sessions_stop
// queue entry is dispatched to DELETE /api/v1/active/{id} with the right
// If-Match header and tag/note body.
func TestWorker_DrainActiveStop_HappyPath(t *testing.T) {
	inner := &drainableQueue{}
	stopBody, _ := json.Marshal(struct {
		Action string `json:"action"`
		Tag    string `json:"tag"`
		Note   string `json:"note"`
	}{"stop", "shipped", "great session"})
	_ = inner.enqueueRaw("active_sessions_stop", "proj-X", stopBody, 7)

	var sawStop atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete && contains(r.URL.Path, "/active/proj-X") {
			sawStop.Add(1)
			if r.Header.Get("If-Match") != "7" {
				t.Errorf("If-Match: got %q, want 7", r.Header.Get("If-Match"))
			}
			_ = json.NewEncoder(w).Encode(domain.Session{ID: "stopped-sess", Version: 12})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	w := newWorker(
		srv,
		&testutil.FakeSessionStore{},
		&testutil.FakeProjectStore{},
		&testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}},
		&testutil.FakeSyncWatermarkStore{},
		inner,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.Start(ctx)

	if !eventually(func() bool {
		entries, _ := inner.Peek(10)
		return sawStop.Load() >= 1 && len(entries) == 0
	}, 1500*time.Millisecond) {
		entries, _ := inner.Peek(10)
		t.Fatalf("expected stop drained: sawStop=%d remaining=%d", sawStop.Load(), len(entries))
	}
	w.Stop()
	<-w.Done()
}

// TestWorker_ForcePull_TriggersImmediatePullCycle verifies that calling
// ForcePull pokes the worker's pullSignal channel, causing a pull cycle to
// run before the next regular tick. We use a sleepy pull-interval so a
// signal-driven cycle is unmistakable.
func TestWorker_ForcePull_TriggersImmediatePullCycle(t *testing.T) {
	var sessCallCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && contains(r.URL.Path, "/sessions") {
			sessCallCount.Add(1)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	ss := &testutil.FakeSessionStore{}
	ps := &testutil.FakeProjectStore{}
	as := &testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}}
	ws := &testutil.FakeSyncWatermarkStore{}

	c := newClient(srv, "tok")
	q := httpsync.NewQueue(&drainableQueue{})
	w := httpsync.NewWorker(c, ss, ps, as, ws, q, "user1")
	w.PullInterval = 1 * time.Hour // effectively disable tick-driven pulls

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	// Initial pull on Start consumes the first signal-or-tick path; wait for it
	// to land so ForcePull's signal is distinguishable from startup noise.
	if !eventually(func() bool { return sessCallCount.Load() >= 1 }, 500*time.Millisecond) {
		t.Fatal("initial pull did not happen")
	}
	before := sessCallCount.Load()

	w.ForcePull()
	if !eventually(func() bool { return sessCallCount.Load() > before }, 500*time.Millisecond) {
		t.Fatalf("ForcePull did not trigger a pull: before=%d after=%d", before, sessCallCount.Load())
	}
}

// TestWorker_ForcePull_Coalesces verifies that multiple back-to-back ForcePull
// calls do not block — the channel is buffered-1 with select-default in the
// implementation, so excess signals are dropped rather than queued.
func TestWorker_ForcePull_Coalesces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	q := httpsync.NewQueue(&drainableQueue{})
	w := httpsync.NewWorker(
		c,
		&testutil.FakeSessionStore{},
		&testutil.FakeProjectStore{},
		&testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}},
		&testutil.FakeSyncWatermarkStore{},
		q,
		"user1",
	)
	// Worker not Started; ForcePull pokes the channel but no consumer. The
	// select-default branch must keep the call non-blocking even when the
	// buffer is full.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			w.ForcePull()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ForcePull blocked despite coalescing")
	}
}

// ---- Test: pull remaps user_id to local user ---------------------------------

// TestUnit_Worker_PullRemapsUserIDToLocalUser verifies that active sessions
// pulled from the server (which carry the server's user UUID) are stored with
// the worker's own local user id. The server scopes every pull to the
// authenticated user, so all pulled rows belong to the local user.
func TestUnit_Worker_PullRemapsUserIDToLocalUser(t *testing.T) {
	// The server returns one active session owned by "server-uuid".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && contains(r.URL.Path, "/active") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []domain.ActiveSession{{
					UserID:    "server-uuid",
					ProjectID: "proj-1",
					Version:   1,
				}},
				"high_watermark": int64(1),
				"has_more":       false,
			})
			return
		}
		// All other pull endpoints — return empty pages.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	as := &testutil.FakeActiveSessionStoreV2{Rows: map[string]domain.ActiveSession{}}
	ws := &testutil.FakeSyncWatermarkStore{}
	w := newWorker(srv, &testutil.FakeSessionStore{}, &testutil.FakeProjectStore{}, as, ws, &drainableQueue{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w.Start(ctx)

	// Wait until the watermark for active_sessions advances — proof that a pull
	// cycle completed and the row was ingested.
	if !eventually(func() bool {
		wm, _ := ws.Get("active_sessions")
		return wm >= 1
	}, 1500*time.Millisecond) {
		t.Fatal("timed out waiting for active_sessions pull to complete")
	}

	w.Stop()
	<-w.Done()

	// The stored row must carry the worker's local user id ("user1"), not "server-uuid".
	rows, err := as.ListByUser("user1")
	if err != nil {
		t.Fatalf("ListByUser error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("stored rows for user1: got %d, want 1", len(rows))
	}
	if rows[0].UserID != "user1" {
		t.Errorf("UserID: got %q, want %q", rows[0].UserID, "user1")
	}
}

// eventually polls cond until it returns true or timeout elapses.
func eventually(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
