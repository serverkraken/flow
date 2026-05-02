package testutil

import (
	"context"
	"sync"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// FakeNotebookInit is an in-memory ports.NotebookInitializer for use-case
// tests. It records calls and respects the *Err overrides for failure
// branches.
type FakeNotebookInit struct {
	mu sync.Mutex

	Initialized bool
	Snapshots   []string

	IsRepoErr     error
	InitErr       error
	HasChangesErr error
	SnapshotErr   error

	// Reported state — tests can flip these to drive use-case behaviour.
	IsRepoValue     bool
	HasChangesValue bool
}

// IsRepo implements ports.NotebookInitializer.
func (f *FakeNotebookInit) IsRepo(_ context.Context, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.IsRepoErr != nil {
		return false, f.IsRepoErr
	}
	return f.IsRepoValue, nil
}

// Init implements ports.NotebookInitializer.
func (f *FakeNotebookInit) Init(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.InitErr != nil {
		return f.InitErr
	}
	f.Initialized = true
	f.IsRepoValue = true
	return nil
}

// HasUncommittedChanges implements ports.NotebookInitializer.
func (f *FakeNotebookInit) HasUncommittedChanges(_ context.Context, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.HasChangesErr != nil {
		return false, f.HasChangesErr
	}
	return f.HasChangesValue, nil
}

// Snapshot implements ports.NotebookInitializer.
func (f *FakeNotebookInit) Snapshot(_ context.Context, _ string, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SnapshotErr != nil {
		return f.SnapshotErr
	}
	f.Snapshots = append(f.Snapshots, message)
	f.HasChangesValue = false
	return nil
}

var _ ports.NotebookInitializer = (*FakeNotebookInit)(nil)
