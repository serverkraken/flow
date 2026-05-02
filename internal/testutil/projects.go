package testutil

import "github.com/serverkraken/flow/internal/domain"

// FakeProjectScanner returns the project list supplied by the test.
// Set Names for a quick path-less list (Path defaults to "/tmp/<name>"),
// or set Projects directly for full control.
type FakeProjectScanner struct {
	Names    []string
	Projects []domain.Project
	Err      error
}

func (f *FakeProjectScanner) List() ([]domain.Project, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if f.Projects != nil {
		out := make([]domain.Project, len(f.Projects))
		copy(out, f.Projects)
		return out, nil
	}
	out := make([]domain.Project, len(f.Names))
	for i, n := range f.Names {
		out[i] = domain.Project{Name: n, Path: "/tmp/" + n}
	}
	return out, nil
}
