package usecase

import (
	"errors"

	"github.com/serverkraken/flow/internal/ports"
)

// ErrSyncWorkerNotWired is returned by SyncStatus stub methods that require
// the real sync worker (Task 29). Callers should surface this as an
// informational message rather than a hard error.
var ErrSyncWorkerNotWired = errors.New("flow: sync worker not yet wired (Task 32)")

// knownResources is the canonical list of resources tracked by the
// SyncWatermarkStore. Populated from the domain model spec (M2f).
var knownResources = []string{
	"projects",
	"sessions",
	"active_sessions",
	"users",
	"repos",
	"repo_notes",
}

// SyncStatus provides a minimal implementation of ports.SyncController.
// It surfaces WriteQueue length and per-resource watermarks via the Status()
// method. ForcePull, AcceptServerVersion, and OverwriteServerVersion return
// ErrSyncWorkerNotWired until the real worker is wired in Task 29.
type SyncStatus struct {
	queue      ports.WriteQueue
	watermarks ports.SyncWatermarkStore
}

// compile-time assertion: SyncStatus must satisfy ports.SyncController.
var _ ports.SyncController = (*SyncStatus)(nil)

// NewSyncStatus constructs a SyncStatus use case.
func NewSyncStatus(queue ports.WriteQueue, watermarks ports.SyncWatermarkStore) *SyncStatus {
	return &SyncStatus{queue: queue, watermarks: watermarks}
}

// Status returns the current WriteQueue length and per-resource watermarks.
// LastPullAt and LastPullError are empty until the real sync worker runs.
func (s *SyncStatus) Status() (ports.SyncStatus, error) {
	// Peek with a large limit to count all pending entries.
	entries, err := s.queue.Peek(10000)
	if err != nil {
		return ports.SyncStatus{}, err
	}

	wm := make(map[string]int64, len(knownResources))
	for _, resource := range knownResources {
		val, err := s.watermarks.Get(resource)
		if err != nil {
			return ports.SyncStatus{}, err
		}
		wm[resource] = val
	}

	return ports.SyncStatus{
		QueueLen:      len(entries),
		LastPullAt:    "",
		LastPullError: "",
		Watermarks:    wm,
	}, nil
}

// ForcePull is a stub — returns ErrSyncWorkerNotWired until Task 29.
func (s *SyncStatus) ForcePull() error {
	return ErrSyncWorkerNotWired
}

// AcceptServerVersion is a stub — returns ErrSyncWorkerNotWired until Task 29.
func (s *SyncStatus) AcceptServerVersion(_ int64) error {
	return ErrSyncWorkerNotWired
}

// OverwriteServerVersion is a stub — returns ErrSyncWorkerNotWired until Task 29.
func (s *SyncStatus) OverwriteServerVersion(_ int64) error {
	return ErrSyncWorkerNotWired
}
