// Package testutil provides in-memory implementations of the ports
// interfaces for use-case tests. It is excluded from the coverage gate — the
// fakes are test infrastructure, not production code.
package testutil

import (
	"context"
	"sync"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// FakeEditor records Edit calls without launching a real editor.
type FakeEditor struct {
	mu    sync.Mutex
	Calls []string
	Err   error
}

// Edit implements ports.Editor.
func (f *FakeEditor) Edit(_ context.Context, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, path)
	return f.Err
}

var _ ports.Editor = (*FakeEditor)(nil)
