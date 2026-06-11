package ports

// WriteQueue durably buffers local mutations for delivery to the server.
// Entries survive process restarts; the sync worker drains them in FIFO order.
//
// # Retry semantics
//
// Peek MUST filter out entries whose next_retry_at lies in the future
// (compared against time.Now()). Entries with empty next_retry_at are
// always eligible.
//
// SetError records an error message without bumping attempt — used for
// 4xx classification telemetry where the entry is about to be Removed.
// SetRetry bumps attempt and persists nextRetryAt — used by the
// sync worker on every transient failure so the next drain skips
// the row until the backoff elapses.
type WriteQueue interface {
	Enqueue(resource, rowID string, payload []byte, expectedVersion int64) (seq int64, err error)
	Peek(limit int) ([]WriteQueueEntry, error)
	Remove(seq int64) error
	SetError(seq int64, errMsg string) error
	SetRetry(seq int64, errMsg string, nextRetryAt string) error
}

// WriteQueueEntry is one durably buffered mutation awaiting delivery.
type WriteQueueEntry struct {
	Seq             int64
	Resource        string
	RowID           string
	Payload         []byte
	ExpectedVersion int64
	EnqueuedAt      string
	LastError       string
	Attempt         int
	NextRetryAt     string // RFC3339; empty means "eligible immediately"
}
