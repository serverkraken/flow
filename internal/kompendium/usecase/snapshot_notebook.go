package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

const defaultSnapshotMessage = "kompendium snapshot"

// SnapshotNotebook stages and commits any pending changes in the notebook,
// skipping the commit when the working tree is already clean.
type SnapshotNotebook struct {
	Rooter ports.NotebookRooter
	Git    ports.NotebookInitializer
}

// NewSnapshotNotebook wires the use case with its required ports.
func NewSnapshotNotebook(rooter ports.NotebookRooter, git ports.NotebookInitializer) *SnapshotNotebook {
	return &SnapshotNotebook{Rooter: rooter, Git: git}
}

// SnapshotNotebookInput carries the optional commit message override.
type SnapshotNotebookInput struct {
	Message string
}

// SnapshotNotebookOutput tells the caller whether anything was committed.
type SnapshotNotebookOutput struct {
	HadChanges bool
	Committed  bool
}

// Execute checks for pending changes and commits them with the supplied
// message (or the default when empty). On a clean tree it is a no-op.
func (u *SnapshotNotebook) Execute(ctx context.Context, in SnapshotNotebookInput) (SnapshotNotebookOutput, error) {
	root := u.Rooter.Root()
	dirty, err := u.Git.HasUncommittedChanges(ctx, root)
	if err != nil {
		return SnapshotNotebookOutput{}, fmt.Errorf("has-changes: %w", err)
	}
	if !dirty {
		return SnapshotNotebookOutput{HadChanges: false, Committed: false}, nil
	}
	msg := in.Message
	if msg == "" {
		msg = defaultSnapshotMessage
	}
	if err := u.Git.Snapshot(ctx, root, msg); err != nil {
		return SnapshotNotebookOutput{}, fmt.Errorf("snapshot: %w", err)
	}
	return SnapshotNotebookOutput{HadChanges: true, Committed: true}, nil
}
