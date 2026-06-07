package testutil

import "github.com/serverkraken/flow/internal/ports"

var _ ports.WriteQueue = (*FakeWriteQueue)(nil)

// FakeWriteQueue is a no-op in-memory ports.WriteQueue for tests. It
// accepts all Enqueue calls and returns monotonically increasing sequence
// numbers. Peek/Remove/SetError are no-ops (tests that need to assert queue
// contents can inspect the Entries slice directly).
type FakeWriteQueue struct {
	Entries []FakeWriteQueueEntry
	Err     error
	nextSeq int64
}

// FakeWriteQueueEntry records one enqueued mutation.
type FakeWriteQueueEntry struct {
	Seq             int64
	Resource        string
	RowID           string
	Payload         []byte
	ExpectedVersion int64
}

// Enqueue implements ports.WriteQueue.
func (f *FakeWriteQueue) Enqueue(resource, rowID string, payload []byte, expectedVersion int64) (int64, error) {
	if f.Err != nil {
		return 0, f.Err
	}
	f.nextSeq++
	f.Entries = append(f.Entries, FakeWriteQueueEntry{
		Seq:             f.nextSeq,
		Resource:        resource,
		RowID:           rowID,
		Payload:         payload,
		ExpectedVersion: expectedVersion,
	})
	return f.nextSeq, nil
}

// Peek implements ports.WriteQueue. Returns nothing — tests that need entries
// can read f.Entries directly.
func (f *FakeWriteQueue) Peek(_ int) ([]ports.WriteQueueEntry, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return nil, nil
}

// Remove implements ports.WriteQueue.
func (f *FakeWriteQueue) Remove(_ int64) error { return f.Err }

// SetError implements ports.WriteQueue.
func (f *FakeWriteQueue) SetError(_ int64, _ string) error { return f.Err }

// SetRetry implements ports.WriteQueue.
func (f *FakeWriteQueue) SetRetry(_ int64, _ string, _ string) error { return f.Err }
