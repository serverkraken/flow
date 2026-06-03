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

// ForcePuller is a narrow interface satisfied by httpsync.Worker.
// It lets SyncStatus delegate ForcePull to the real worker without
// importing the httpsync package (avoids cycle: usecase → httpsync → ports → usecase).
type ForcePuller interface {
	ForcePull()
}

// SyncStatus provides a minimal implementation of ports.SyncController.
// It surfaces WriteQueue length and per-resource watermarks via the Status()
// method. When puller is non-nil, ForcePull delegates to it; otherwise it
// returns ErrSyncWorkerNotWired. AcceptServerVersion and OverwriteServerVersion
// return ErrSyncWorkerNotWired until a future task wires queue manipulation.
type SyncStatus struct {
	queue      ports.WriteQueue
	watermarks ports.SyncWatermarkStore
	puller     ForcePuller // optional — nil-tolerant
}

// compile-time assertion: SyncStatus must satisfy ports.SyncController.
var _ ports.SyncController = (*SyncStatus)(nil)

// NewSyncStatus constructs a SyncStatus use case. Pass nil for puller to keep
// ForcePull as a stub (pre-wiring state).
func NewSyncStatus(queue ports.WriteQueue, watermarks ports.SyncWatermarkStore) *SyncStatus {
	return &SyncStatus{queue: queue, watermarks: watermarks}
}

// WithForcePuller attaches the real worker so ForcePull delegates instead of
// returning ErrSyncWorkerNotWired. Called by the composition root after the
// worker is constructed.
func (s *SyncStatus) WithForcePuller(p ForcePuller) {
	s.puller = p
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

// ForcePull signals the background worker to run an immediate pull cycle.
// Returns ErrSyncWorkerNotWired if no worker has been wired via WithForcePuller.
func (s *SyncStatus) ForcePull() error {
	if s.puller == nil {
		return ErrSyncWorkerNotWired
	}
	s.puller.ForcePull()
	return nil
}

// AcceptServerVersion is a stub — returns ErrSyncWorkerNotWired until Task 29.
func (s *SyncStatus) AcceptServerVersion(_ int64) error {
	return ErrSyncWorkerNotWired
}

// OverwriteServerVersion is a stub — returns ErrSyncWorkerNotWired until Task 29.
func (s *SyncStatus) OverwriteServerVersion(_ int64) error {
	return ErrSyncWorkerNotWired
}
