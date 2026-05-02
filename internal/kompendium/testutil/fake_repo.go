package testutil

import (
	"context"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// FakeRepoDetector returns either a pre-configured RepoInfo or Err. Detect
// ignores cwd — callers choose the response by populating the fields.
type FakeRepoDetector struct {
	Info ports.RepoInfo
	Err  error
}

// Detect implements ports.RepoDetector.
func (f *FakeRepoDetector) Detect(_ context.Context, _ string) (ports.RepoInfo, error) {
	if f.Err != nil {
		return ports.RepoInfo{}, f.Err
	}
	return f.Info, nil
}

var _ ports.RepoDetector = (*FakeRepoDetector)(nil)
