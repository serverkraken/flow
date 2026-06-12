package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// InitNotebook makes the notebook directory a git repository, creating the
// initial commit. Idempotent — already-initialised notebooks are reported
// as such without touching the existing repo.
type InitNotebook struct {
	Rooter ports.NotebookRooter
	Git    ports.NotebookInitializer
}

// NewInitNotebook wires the use case with its required ports.
func NewInitNotebook(rooter ports.NotebookRooter, git ports.NotebookInitializer) *InitNotebook {
	return &InitNotebook{Rooter: rooter, Git: git}
}

// InitNotebookOutput reports the resolved root and whether the directory
// was already a git repo.
type InitNotebookOutput struct {
	Root               string
	AlreadyInitialized bool
}

// Execute checks IsRepo first; if false, calls Init.
func (u *InitNotebook) Execute(ctx context.Context) (InitNotebookOutput, error) {
	root := u.Rooter.Root()
	already, err := u.Git.IsRepo(ctx, root)
	if err != nil {
		return InitNotebookOutput{}, fmt.Errorf("is-repo: %w", err)
	}
	if already {
		return InitNotebookOutput{Root: root, AlreadyInitialized: true}, nil
	}
	if err := u.Git.Init(ctx, root); err != nil {
		return InitNotebookOutput{}, fmt.Errorf("init: %w", err)
	}
	return InitNotebookOutput{Root: root, AlreadyInitialized: false}, nil
}
