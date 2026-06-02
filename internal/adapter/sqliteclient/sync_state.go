package sqliteclient

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/serverkraken/flow/internal/ports"
)

// SyncState implements ports.SyncWatermarkStore against the SQLite sync_state table.
type SyncState struct {
	store *Store
}

// compile-time interface assertion
var _ ports.SyncWatermarkStore = (*SyncState)(nil)

// NewSyncState constructs a SyncState sub-adapter backed by store.
func NewSyncState(store *Store) *SyncState { return &SyncState{store: store} }

// Get returns the watermark for the given resource. Returns 0 and nil when no
// watermark has been recorded yet — a missing entry is not an error.
func (s *SyncState) Get(resource string) (int64, error) {
	var watermark int64
	err := s.store.DB().QueryRow(
		`SELECT watermark FROM sync_state WHERE resource = ?`, resource,
	).Scan(&watermark)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("sqliteclient.SyncState.Get: %w", err)
	}
	return watermark, nil
}

// Set persists the watermark for the given resource, creating or updating the row.
func (s *SyncState) Set(resource string, watermark int64) error {
	_, err := s.store.DB().Exec(
		`INSERT INTO sync_state (resource, watermark) VALUES (?, ?)
		 ON CONFLICT(resource) DO UPDATE SET watermark = excluded.watermark`,
		resource, watermark,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.SyncState.Set: %w", err)
	}
	return nil
}
