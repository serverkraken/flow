package testutil

import (
	"context"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// FakeNotebookRemote is the shared in-test ports.NotebookRemote used
// by use-case and CLI tests. Lives in testutil so both call sites
// pick up new methods automatically when NotebookRemote grows — the
// previous in-line fakeRemote structs in sync_notebook_test.go and
// cli_test.go drifted independently.
type FakeNotebookRemote struct {
	URL      string
	GetErr   error
	SetURL   string
	SetErr   error
	SyncRoot string
	Stats    ports.SyncStats
	SyncErr  error
}

// GetRemote implements ports.NotebookRemote.
func (f *FakeNotebookRemote) GetRemote(_ context.Context, _ string) (string, error) {
	if f.GetErr != nil {
		return "", f.GetErr
	}
	return f.URL, nil
}

// SetRemote implements ports.NotebookRemote.
func (f *FakeNotebookRemote) SetRemote(_ context.Context, _, url string) error {
	f.SetURL = url
	return f.SetErr
}

// Sync implements ports.NotebookRemote.
func (f *FakeNotebookRemote) Sync(_ context.Context, root string) (ports.SyncStats, error) {
	f.SyncRoot = root
	return f.Stats, f.SyncErr
}

var _ ports.NotebookRemote = (*FakeNotebookRemote)(nil)
