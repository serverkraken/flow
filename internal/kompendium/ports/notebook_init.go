package ports

import "context"

// NotebookInitializer manages the notebook directory's git lifecycle:
// turning it into a repo, checking for pending edits, and snapshotting them
// into commits. It deliberately knows nothing about notes, IDs, or
// frontmatter — only filesystem roots and commit messages.
type NotebookInitializer interface {
	// IsRepo reports whether root is already a git working tree.
	IsRepo(ctx context.Context, root string) (bool, error)
	// Init initialises root as a git repository and creates an initial commit.
	Init(ctx context.Context, root string) error
	// HasUncommittedChanges reports whether the working tree differs from HEAD.
	HasUncommittedChanges(ctx context.Context, root string) (bool, error)
	// Snapshot stages every change under root and creates a commit with the
	// supplied message. Implementations may allow empty commits so the
	// caller doesn't have to second-guess the working tree's state.
	Snapshot(ctx context.Context, root, message string) error
}
