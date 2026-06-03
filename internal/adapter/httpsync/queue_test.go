package httpsync_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpsync"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ---- drainableQueue — a full in-memory ports.WriteQueue for Drain tests ----

type drainEntry struct {
	seq             int64
	resource        string
	rowID           string
	payload         []byte
	expectedVersion int64
	lastError       string
	removed         bool
}

// drainableQueue is a concurrency-safe in-memory ports.WriteQueue used in
// both queue_test.go and worker_test.go. A mutex guards all field accesses so
// that the worker goroutine and test goroutine can read/write concurrently
// under -race without false positives.
type drainableQueue struct {
	mu      sync.Mutex
	entries []*drainEntry
	nextSeq int64
}

func (d *drainableQueue) Enqueue(resource, rowID string, payload []byte, expectedVersion int64) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.nextSeq++
	d.entries = append(d.entries, &drainEntry{
		seq:             d.nextSeq,
		resource:        resource,
		rowID:           rowID,
		payload:         payload,
		expectedVersion: expectedVersion,
	})
	return d.nextSeq, nil
}

func (d *drainableQueue) Peek(limit int) ([]ports.WriteQueueEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []ports.WriteQueueEntry
	for _, e := range d.entries {
		if e.removed {
			continue
		}
		if len(out) >= limit {
			break
		}
		out = append(out, ports.WriteQueueEntry{
			Seq:             e.seq,
			Resource:        e.resource,
			RowID:           e.rowID,
			Payload:         e.payload,
			ExpectedVersion: e.expectedVersion,
			LastError:       e.lastError,
		})
	}
	return out, nil
}

func (d *drainableQueue) Remove(seq int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.entries {
		if e.seq == seq {
			e.removed = true
			return nil
		}
	}
	return nil
}

func (d *drainableQueue) SetError(seq int64, errMsg string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.entries {
		if e.seq == seq {
			e.lastError = errMsg
			return nil
		}
	}
	return nil
}

// ---- EnqueueSession ----

func TestQueue_EnqueueSession(t *testing.T) {
	inner := &drainableQueue{}
	q := httpsync.NewQueue(inner)
	s := domain.Session{ID: "s1", Tag: "deep"}
	seq, err := q.EnqueueSession(s, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seq != 1 {
		t.Errorf("seq: got %d, want 1", seq)
	}
	if len(inner.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(inner.entries))
	}
	e := inner.entries[0]
	if e.resource != "sessions" {
		t.Errorf("resource: got %q, want %q", e.resource, "sessions")
	}
	if e.rowID != "s1" {
		t.Errorf("rowID: got %q, want %q", e.rowID, "s1")
	}
	if e.expectedVersion != 3 {
		t.Errorf("expectedVersion: got %d, want 3", e.expectedVersion)
	}
	var decoded domain.Session
	if err := json.Unmarshal(e.payload, &decoded); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if decoded.ID != "s1" || decoded.Tag != "deep" {
		t.Errorf("decoded session mismatch: %+v", decoded)
	}
}

// ---- EnqueueProject ----

func TestQueue_EnqueueProject(t *testing.T) {
	inner := &drainableQueue{}
	q := httpsync.NewQueue(inner)
	p := domain.Project{ID: "p1", Name: "Alpha"}
	seq, err := q.EnqueueProject(p, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seq != 1 {
		t.Errorf("seq: got %d, want 1", seq)
	}
	e := inner.entries[0]
	if e.resource != "projects" {
		t.Errorf("resource: got %q, want %q", e.resource, "projects")
	}
	if e.rowID != "p1" {
		t.Errorf("rowID: got %q, want %q", e.rowID, "p1")
	}
	var decoded domain.Project
	if err := json.Unmarshal(e.payload, &decoded); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if decoded.Name != "Alpha" {
		t.Errorf("decoded project name: got %q, want Alpha", decoded.Name)
	}
}

// ---- EnqueueActiveStart ----

func TestQueue_EnqueueActiveStart(t *testing.T) {
	inner := &drainableQueue{}
	q := httpsync.NewQueue(inner)
	_, err := q.EnqueueActiveStart("p1", "laptop", "", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	e := inner.entries[0]
	if e.resource != "active_sessions" {
		t.Errorf("resource: got %q", e.resource)
	}
	var body map[string]string
	if err := json.Unmarshal(e.payload, &body); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if body["action"] != "start" {
		t.Errorf("action: got %q, want start", body["action"])
	}
	if body["project_id"] != "p1" {
		t.Errorf("project_id: got %q, want p1", body["project_id"])
	}
	if body["started_on_device"] != "laptop" {
		t.Errorf("started_on_device: got %q, want laptop", body["started_on_device"])
	}
}

// ---- EnqueueActiveStop ----

func TestQueue_EnqueueActiveStop(t *testing.T) {
	inner := &drainableQueue{}
	q := httpsync.NewQueue(inner)
	_, err := q.EnqueueActiveStop("p1", 2, "meeting", "standup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	e := inner.entries[0]
	if e.resource != "active_sessions_stop" {
		t.Errorf("resource: got %q, want active_sessions_stop", e.resource)
	}
	var body map[string]string
	if err := json.Unmarshal(e.payload, &body); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if body["action"] != "stop" {
		t.Errorf("action: got %q, want stop", body["action"])
	}
	if body["tag"] != "meeting" {
		t.Errorf("tag: got %q, want meeting", body["tag"])
	}
	if body["note"] != "standup" {
		t.Errorf("note: got %q, want standup", body["note"])
	}
}

// ---- Drain happy path ----

func TestQueue_Drain_HappyPath(t *testing.T) {
	inner := &drainableQueue{}
	q := httpsync.NewQueue(inner)
	// Enqueue 3 sessions directly into the inner queue.
	for i := int64(1); i <= 3; i++ {
		_, _ = inner.Enqueue("sessions", "s"+string(rune('0'+i)), []byte("{}"), 0)
	}

	called := 0
	err := q.Drain(func(_ ports.WriteQueueEntry) (bool, error) {
		called++
		return true, nil // success for all
	})
	if err != nil {
		t.Fatalf("Drain error: %v", err)
	}
	if called != 3 {
		t.Errorf("callback called %d times, want 3", called)
	}
	// All entries should be removed.
	for _, e := range inner.entries {
		if !e.removed {
			t.Errorf("entry seq=%d not removed", e.seq)
		}
	}
}

// ---- Drain with error ----

func TestQueue_Drain_WithError(t *testing.T) {
	inner := &drainableQueue{}
	q := httpsync.NewQueue(inner)
	// 3 entries, second will fail.
	for i := 1; i <= 3; i++ {
		_, _ = inner.Enqueue("sessions", "s", []byte("{}"), 0)
	}
	seqs := []int64{inner.entries[0].seq, inner.entries[1].seq, inner.entries[2].seq}
	cbErr := errors.New("push failed")

	called := 0
	err := q.Drain(func(e ports.WriteQueueEntry) (bool, error) {
		called++
		if e.Seq == seqs[1] {
			return false, cbErr
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("Drain error: %v", err)
	}
	if called != 3 {
		t.Errorf("callback called %d times, want 3", called)
	}
	// Entry 1 and 3 removed; entry 2 has error set.
	if !inner.entries[0].removed {
		t.Error("entry 1 should be removed")
	}
	if inner.entries[1].removed {
		t.Error("entry 2 should NOT be removed")
	}
	if inner.entries[1].lastError != cbErr.Error() {
		t.Errorf("entry 2 lastError: got %q, want %q", inner.entries[1].lastError, cbErr.Error())
	}
	if !inner.entries[2].removed {
		t.Error("entry 3 should be removed")
	}
}

// ---- Drain halts on conflict ----

func TestQueue_Drain_HaltsOnConflict(t *testing.T) {
	inner := &drainableQueue{}
	q := httpsync.NewQueue(inner)
	// 3 entries; first returns (false, nil) = conflict/signal.
	for i := 1; i <= 3; i++ {
		_, _ = inner.Enqueue("sessions", "s", []byte("{}"), 0)
	}

	called := 0
	err := q.Drain(func(_ ports.WriteQueueEntry) (bool, error) {
		called++
		return false, nil // halt
	})
	if err != nil {
		t.Fatalf("Drain error: %v", err)
	}
	if called != 1 {
		t.Errorf("callback called %d times, want 1", called)
	}
	// Nothing removed, nothing errored.
	for _, e := range inner.entries {
		if e.removed {
			t.Errorf("entry seq=%d should not be removed", e.seq)
		}
		if e.lastError != "" {
			t.Errorf("entry seq=%d should have no error, got %q", e.seq, e.lastError)
		}
	}
}
