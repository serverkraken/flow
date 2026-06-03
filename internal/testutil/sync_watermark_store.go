package testutil

import (
	"sync"

	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.SyncWatermarkStore = (*FakeSyncWatermarkStore)(nil)

// FakeSyncWatermarkStore is a concurrency-safe in-memory ports.SyncWatermarkStore
// for tests. Watermarks are stored by resource name and default to 0 when absent.
// A mutex guards all accesses so that the worker goroutine and test goroutine can
// call Get/Set concurrently without triggering the race detector.
type FakeSyncWatermarkStore struct {
	mu    sync.Mutex
	marks map[string]int64
	Err   error
}

// Get returns the stored watermark for resource, or 0 if not yet set.
func (f *FakeSyncWatermarkStore) Get(resource string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return 0, f.Err
	}
	if f.marks == nil {
		return 0, nil
	}
	return f.marks[resource], nil
}

// Set stores the watermark for resource.
func (f *FakeSyncWatermarkStore) Set(resource string, watermark int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	if f.marks == nil {
		f.marks = map[string]int64{}
	}
	f.marks[resource] = watermark
	return nil
}
