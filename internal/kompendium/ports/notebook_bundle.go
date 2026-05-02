package ports

import "context"

// NotebookBundler exports and imports the notebook's git history as a
// single git-bundle file. It is the "incremental + mergeable" transfer
// path: history travels (so the recipient can pull future bundles
// incrementally), and conflicts surface as ordinary git merge conflicts
// that the user resolves with their editor.
//
// See CLAUDE.md section 11 for the trade-off vs. ports.TarSnapshot.
type NotebookBundler interface {
	ExportBundle(ctx context.Context, root, outPath string) error
	ImportBundle(ctx context.Context, root, bundlePath string) error
}
