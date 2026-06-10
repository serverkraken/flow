package httpsync_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

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
	attempt         int
	nextRetryAt     string // RFC3339; empty == eligible
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
	now := time.Now().UTC().Format(time.RFC3339)
	var out []ports.WriteQueueEntry
	for _, e := range d.entries {
		if e.removed {
			continue
		}
		// Skip entries whose backoff hasn't elapsed yet.
		if e.nextRetryAt != "" && e.nextRetryAt > now {
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
			Attempt:         e.attempt,
			NextRetryAt:     e.nextRetryAt,
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

func (d *drainableQueue) SetRetry(seq int64, errMsg string, nextRetryAt string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.entries {
		if e.seq == seq {
			e.lastError = errMsg
			e.attempt++
			e.nextRetryAt = nextRetryAt
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

// fastBackoff is a tiny non-jittered Backoff used by queue tests so SetRetry
// timestamps are predictable enough for an "in the future" assertion.
var fastBackoff = httpsync.Backoff{
	Base:   100 * time.Millisecond,
	Max:    time.Second,
	Factor: 2.0,
	Jitter: -1,
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
	err := q.Drain(func(_ ports.WriteQueueEntry) (httpsync.DrainAction, error) {
		called++
		return httpsync.DrainAck, nil // success for all
	}, fastBackoff)
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

// ---- Drain with retry ----

func TestQueue_Drain_WithRetry(t *testing.T) {
	inner := &drainableQueue{}
	q := httpsync.NewQueue(inner)
	// 3 entries, second will be marked DrainRetry.
	for i := 1; i <= 3; i++ {
		_, _ = inner.Enqueue("sessions", "s", []byte("{}"), 0)
	}
	seqs := []int64{inner.entries[0].seq, inner.entries[1].seq, inner.entries[2].seq}
	cbErr := errors.New("transient push failure")

	called := 0
	err := q.Drain(func(e ports.WriteQueueEntry) (httpsync.DrainAction, error) {
		called++
		if e.Seq == seqs[1] {
			return httpsync.DrainRetry, cbErr
		}
		return httpsync.DrainAck, nil
	}, fastBackoff)
	if err != nil {
		t.Fatalf("Drain error: %v", err)
	}
	if called != 3 {
		t.Errorf("callback called %d times, want 3", called)
	}
	// Entry 1 and 3 removed; entry 2 still pending with retry scheduled.
	if !inner.entries[0].removed {
		t.Error("entry 1 should be removed")
	}
	if inner.entries[1].removed {
		t.Error("entry 2 should NOT be removed (retry pending)")
	}
	if inner.entries[1].attempt != 1 {
		t.Errorf("entry 2 attempt: got %d, want 1", inner.entries[1].attempt)
	}
	if inner.entries[1].nextRetryAt == "" {
		t.Error("entry 2 nextRetryAt should be set")
	}
	if inner.entries[1].lastError != cbErr.Error() {
		t.Errorf("entry 2 lastError: got %q, want %q", inner.entries[1].lastError, cbErr.Error())
	}
	if !inner.entries[2].removed {
		t.Error("entry 3 should be removed")
	}
}

// ---- Drain ack with permanent error ----

func TestQueue_Drain_PermanentError_Acks(t *testing.T) {
	inner := &drainableQueue{}
	q := httpsync.NewQueue(inner)
	_, _ = inner.Enqueue("sessions", "s1", []byte("{}"), 0)

	permErr := errors.New("permanent: 422 unprocessable")
	err := q.Drain(func(_ ports.WriteQueueEntry) (httpsync.DrainAction, error) {
		return httpsync.DrainAck, permErr
	}, fastBackoff)
	if err != nil {
		t.Fatalf("Drain error: %v", err)
	}
	if !inner.entries[0].removed {
		t.Error("permanent-failure entry should be removed (Ack)")
	}
	if inner.entries[0].lastError != permErr.Error() {
		t.Errorf("lastError: got %q, want %q", inner.entries[0].lastError, permErr.Error())
	}
}

// ---- Drain halts on conflict ----

func TestQueue_Drain_HaltsOnConflict(t *testing.T) {
	inner := &drainableQueue{}
	q := httpsync.NewQueue(inner)
	// 3 entries; first returns DrainHalt — drain must stop.
	for i := 1; i <= 3; i++ {
		_, _ = inner.Enqueue("sessions", "s", []byte("{}"), 0)
	}

	called := 0
	err := q.Drain(func(_ ports.WriteQueueEntry) (httpsync.DrainAction, error) {
		called++
		return httpsync.DrainHalt, nil
	}, fastBackoff)
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
