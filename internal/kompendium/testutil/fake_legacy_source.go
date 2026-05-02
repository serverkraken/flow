package testutil

import (
	"context"
	"sync"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// FakeLegacySource is an in-memory ports.LegacySource for use-case tests.
// Tests populate Dailies and Projects directly; the *Err fields force the
// matching method to surface that error.
type FakeLegacySource struct {
	mu sync.Mutex

	Dailies  []ports.LegacyDaily
	Projects []ports.LegacyProject

	DailyErr   error
	ProjectErr error
}

// ListDailyNotes implements ports.LegacySource.
func (f *FakeLegacySource) ListDailyNotes(_ context.Context, _ string) ([]ports.LegacyDaily, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.DailyErr != nil {
		return nil, f.DailyErr
	}
	return f.Dailies, nil
}

// ListProjectNotes implements ports.LegacySource.
func (f *FakeLegacySource) ListProjectNotes(_ context.Context, _ string) ([]ports.LegacyProject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ProjectErr != nil {
		return nil, f.ProjectErr
	}
	return f.Projects, nil
}

var _ ports.LegacySource = (*FakeLegacySource)(nil)
