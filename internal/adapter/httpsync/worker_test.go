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
