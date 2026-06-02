package sqliteclient

import (
	"testing"
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
