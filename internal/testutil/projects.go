package testutil

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.SourceDirScanner = (*FakeProjectScanner)(nil)

// FakeProjectScanner returns the source-dir list supplied by the test.
// Set Names for a quick path-less list (Path defaults to "/tmp/<name>"),
// or set Projects directly for full control.
type FakeProjectScanner struct {
	Names    []string
	Projects []domain.SourceDir
	Err      error
}

func (f *FakeProjectScanner) List() ([]domain.SourceDir, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if f.Projects != nil {
		out := make([]domain.SourceDir, len(f.Projects))
		copy(out, f.Projects)
		return out, nil
	}
	out := make([]domain.SourceDir, len(f.Names))
	for i, n := range f.Names {
		out[i] = domain.SourceDir{Name: n, Path: "/tmp/" + n}
	}
	return out, nil
}
