package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// SyncNotebook is the round-trip use case: pull --rebase --autostash,
// then push. The single command that moves notebook state between
// machines. Working-tree changes are auto-stashed around the pull so
// the user doesn't need to think about a clean tree first; uncommitted
// changes are not pushed (run snapshot first to make them travel).
type SyncNotebook struct {
	Store  ports.NoteStore
	Remote ports.NotebookRemote
}

// NewSyncNotebook wires the use case with its required ports.
func NewSyncNotebook(store ports.NoteStore, remote ports.NotebookRemote) *SyncNotebook {
	return &SyncNotebook{Store: store, Remote: remote}
}

// SyncNotebookOutput reports the resolved root and what happened.
type SyncNotebookOutput struct {
	Root  string
	Stats ports.SyncStats
}

// Execute runs the pull/push round-trip on the notebook root.
func (u *SyncNotebook) Execute(ctx context.Context) (SyncNotebookOutput, error) {
	root := u.Store.Root()
	stats, err := u.Remote.Sync(ctx, root)
	if err != nil {
		return SyncNotebookOutput{Root: root, Stats: stats}, fmt.Errorf("sync: %w", err)
	}
	return SyncNotebookOutput{Root: root, Stats: stats}, nil
}
