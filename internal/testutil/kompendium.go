package testutil

import "github.com/serverkraken/flow/internal/ports"

var _ ports.NoteLauncher = (*FakeNoteLauncher)(nil)

// FakeNoteLauncher records every Open invocation as the note ID,
// prefixed with "open:" so a single Calls slice tells the test what
// happened in order. The pre-glow-migration View() shape is gone; the
// integrated renderer hosts read-only views in-process now.
type FakeNoteLauncher struct {
	Calls []string
	Err   error
}

func (f *FakeNoteLauncher) Open(id string) error {
	f.Calls = append(f.Calls, "open:"+id)
	return f.Err
}
