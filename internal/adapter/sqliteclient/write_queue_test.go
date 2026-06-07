package sqliteclient

import (
	"testing"
	"time"
)

func TestUnit_WriteQueue_FIFO_Order(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	wq := NewWriteQueue(store)

	payloads := []string{`{"a":1}`, `{"b":2}`, `{"c":3}`}
	var seqs []int64
	for i, payload := range payloads {
		seq, err := wq.Enqueue("sessions", "row-"+string(rune('a'+i)), []byte(payload), int64(i))
		if err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
		seqs = append(seqs, seq)
	}

	// Seqs must be strictly monotonically increasing.
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Errorf("seq[%d]=%d not > seq[%d]=%d", i, seqs[i], i-1, seqs[i-1])
		}
	}

	entries, err := wq.Peek(10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// FIFO: first enqueued must be first in Peek.
	for i, entry := range entries {
		if entry.Seq != seqs[i] {
			t.Errorf("entry[%d].Seq = %d, want %d", i, entry.Seq, seqs[i])
		}
		if string(entry.Payload) != payloads[i] {
			t.Errorf("entry[%d].Payload = %q, want %q", i, string(entry.Payload), payloads[i])
		}
	}
}

func TestUnit_WriteQueue_Remove_DropsRow(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	wq := NewWriteQueue(store)

	seq, err := wq.Enqueue("projects", "proj-1", []byte(`{}`), 0)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := wq.Remove(seq); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	entries, err := wq.Peek(10)
	if err != nil {
		t.Fatalf("Peek after Remove: %v", err)
	}
	for _, e := range entries {
		if e.Seq == seq {
			t.Errorf("removed entry seq=%d still present", seq)
		}
	}
}

func TestUnit_WriteQueue_SetError_RecordsMessage(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	wq := NewWriteQueue(store)

	seq, err := wq.Enqueue("sessions", "row-err", []byte(`{"x":1}`), 1)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := wq.SetError(seq, "connection timeout"); err != nil {
		t.Fatalf("SetError: %v", err)
	}

	entries, err := wq.Peek(10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Seq == seq {
			found = true
			if e.LastError != "connection timeout" {
				t.Errorf("LastError: got %q, want %q", e.LastError, "connection timeout")
			}
		}
	}
	if !found {
		t.Errorf("entry seq=%d not found after SetError", seq)
	}
}

func TestUnit_WriteQueue_Peek_LimitHonored(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	wq := NewWriteQueue(store)

	for i := 0; i < 5; i++ {
		if _, err := wq.Enqueue("sessions", "row", []byte(`{}`), int64(i)); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	entries, err := wq.Peek(3)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries with limit=3, got %d", len(entries))
	}
}

// TestUnit_WriteQueue_SetRetry_BumpsAttempt verifies that SetRetry persists
// the error message, bumps the attempt counter, and stamps next_retry_at so
// the row can be discriminated by Peek on the next drain.
func TestUnit_WriteQueue_SetRetry_BumpsAttempt(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	wq := NewWriteQueue(store)

	seq, err := wq.Enqueue("sessions", "row-retry", []byte(`{}`), 0)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// First call: attempt 0 → 1, with a past next_retry_at so the row stays eligible.
	pastNext := time.Now().UTC().Add(-time.Second).Format(time.RFC3339)
	if err := wq.SetRetry(seq, "boom", pastNext); err != nil {
		t.Fatalf("SetRetry: %v", err)
	}

	entries, err := wq.Peek(10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	var got *struct {
		attempt   int
		lastError string
		nextRetry string
	}
	for _, e := range entries {
		if e.Seq == seq {
			got = &struct {
				attempt   int
				lastError string
				nextRetry string
			}{e.Attempt, e.LastError, e.NextRetryAt}
		}
	}
	if got == nil {
		t.Fatal("entry not found after SetRetry")
	}
	if got.attempt != 1 {
		t.Errorf("attempt: got %d, want 1", got.attempt)
	}
	if got.lastError != "boom" {
		t.Errorf("lastError: got %q, want %q", got.lastError, "boom")
	}
	if got.nextRetry != pastNext {
		t.Errorf("nextRetryAt: got %q, want %q", got.nextRetry, pastNext)
	}

	// Second call: attempt 1 → 2.
	if err := wq.SetRetry(seq, "still boom", pastNext); err != nil {
		t.Fatalf("SetRetry second call: %v", err)
	}
	entries, _ = wq.Peek(10)
	for _, e := range entries {
		if e.Seq == seq && e.Attempt != 2 {
			t.Errorf("attempt after 2nd SetRetry: got %d, want 2", e.Attempt)
		}
	}
}

// TestUnit_WriteQueue_Peek_FiltersByNextRetryAt verifies that an entry whose
// next_retry_at lies in the future is excluded from Peek, then becomes
// eligible once the timestamp passes.
func TestUnit_WriteQueue_Peek_FiltersByNextRetryAt(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	wq := NewWriteQueue(store)

	seqA, err := wq.Enqueue("sessions", "row-A", []byte(`{}`), 0)
	if err != nil {
		t.Fatalf("Enqueue A: %v", err)
	}
	seqB, err := wq.Enqueue("sessions", "row-B", []byte(`{}`), 0)
	if err != nil {
		t.Fatalf("Enqueue B: %v", err)
	}

	// Park row-A 10 minutes in the future. row-B stays eligible.
	future := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	if err := wq.SetRetry(seqA, "transient", future); err != nil {
		t.Fatalf("SetRetry A: %v", err)
	}

	entries, err := wq.Peek(10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	seenA, seenB := false, false
	for _, e := range entries {
		if e.Seq == seqA {
			seenA = true
		}
		if e.Seq == seqB {
			seenB = true
		}
	}
	if seenA {
		t.Error("row-A should NOT be in Peek result (next_retry_at in future)")
	}
	if !seenB {
		t.Error("row-B should be in Peek result (no next_retry_at)")
	}

	// Move row-A into the past — must become eligible again.
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339)
	if err := wq.SetRetry(seqA, "retried", past); err != nil {
		t.Fatalf("SetRetry A past: %v", err)
	}
	entries, _ = wq.Peek(10)
	seenA = false
	for _, e := range entries {
		if e.Seq == seqA {
			seenA = true
		}
	}
	if !seenA {
		t.Error("row-A should be eligible after next_retry_at moves into the past")
	}
}

// TestUnit_WriteQueue_FreshEnqueue_EligibleImmediately verifies that a freshly
// enqueued row (no SetRetry call) has empty next_retry_at and is returned by
// Peek without any time-window dance.
func TestUnit_WriteQueue_FreshEnqueue_EligibleImmediately(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	wq := NewWriteQueue(store)

	seq, err := wq.Enqueue("sessions", "row-fresh", []byte(`{}`), 0)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	entries, err := wq.Peek(10)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Seq == seq {
			found = true
			if e.Attempt != 0 {
				t.Errorf("Attempt: got %d, want 0", e.Attempt)
			}
			if e.NextRetryAt != "" {
				t.Errorf("NextRetryAt: got %q, want empty", e.NextRetryAt)
			}
		}
	}
	if !found {
		t.Error("fresh entry should be eligible")
	}
}
