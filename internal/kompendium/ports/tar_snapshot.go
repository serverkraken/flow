package ports

import "context"

// ConflictMode controls how an importer reacts to files that already exist
// in the target notebook.
type ConflictMode int

const (
	// ConflictAbort stops on the first conflict and returns an error,
	// leaving the target untouched. Default mode.
	ConflictAbort ConflictMode = iota
	// ConflictNewer keeps the file with the newer mtime; equal mtimes mean
	// "skip" (the existing file wins, since the content is presumed identical).
	ConflictNewer
	// ConflictManual writes the imported file alongside the existing one
	// with a ".imported" suffix so the user can diff and merge by hand.
	ConflictManual
)

// TarSnapshot exports and imports a notebook as a single tar.gz archive of
// its Markdown files. It is the "I just want the notes on the other
// machine" mode — git history travels via ports.NotebookBundler instead
// (see CLAUDE.md section 11 for the trade-off).
type TarSnapshot interface {
	Export(ctx context.Context, sourceRoot, outPath string) error
	Import(ctx context.Context, archive, targetRoot string, mode ConflictMode) error
}
