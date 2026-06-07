package ports

import "github.com/serverkraken/flow/internal/domain"

// ConflictMsg is delivered to listeners when a push gets 409.
type ConflictMsg struct {
	Resource string
	RowID    string
	QueueSeq int64
	Local    any
	Server   any
}

// ConflictListener receives push-conflict notifications from the sync worker.
type ConflictListener interface {
	Conflicts() <-chan ConflictMsg
}

// SyncStatus summarises the current state of the sync worker for display
// in the TUI status bar or the conflict overlay.
type SyncStatus struct {
	QueueLen      int
	LastPullAt    string
	LastPullError string
	Watermarks    map[string]int64
}

// SyncController exposes operational control over the sync worker to the
// TUI and CLI (force-pull, accept/overwrite conflict resolutions).
type SyncController interface {
	Status() (SyncStatus, error)
	ForcePull() error
	AcceptServerVersion(queueSeq int64) error
	OverwriteServerVersion(queueSeq int64) error
}

// SyncWatermarkStore persists per-resource server-side watermarks so the
// client knows how far each pull has progressed.
type SyncWatermarkStore interface {
	Get(resource string) (int64, error)
	Set(resource string, watermark int64) error
}

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
// httpsync.Worker on every transient failure so the next drain skips
// the row until the backoff elapses.
type WriteQueue interface {
	Enqueue(resource, rowID string, payload []byte, expectedVersion int64) (seq int64, err error)
	Peek(limit int) ([]WriteQueueEntry, error)
	Remove(seq int64) error
	SetError(seq int64, errMsg string) error
	SetRetry(seq int64, errMsg string, nextRetryAt string) error
}

// WriteQueueEntry is one durably buffered mutation awaiting delivery.
//
// Attempt and NextRetryAt power the httpsync.Worker's exponential-backoff
// retry policy (Plan F · Task 8). They are zero/empty for a freshly
// enqueued entry; the worker bumps Attempt and recomputes NextRetryAt via
// SetError on each transient failure.
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

var _ = domain.Session{} // ensure domain import always used
