package ports

// Output abstracts where a generated artifact (a Markdown brief, a CSV
// export, a plain-text stats report) goes after the user picks a target.
// The three methods correspond to the worktime menu's output sub-picker:
// Clipboard / tmux-Split with a pager / save to a file.
//
// Implementations are constructed in the composition root and threaded
// into use cases or TUI screens via Deps. The fake at testutil.FakeOutput
// records every call.
type Output interface {
	// Copy puts content into the system clipboard. Tooling differs per
	// OS (pbcopy on macOS, xclip / wl-copy on Linux); the adapter picks
	// what's reachable. Returns an error wrapping the underlying tool
	// failure when no clipboard tool is available.
	Copy(content string) error

	// Pager opens content in a tmux split running viewer. The adapter
	// writes content to a temp file with the given extension and shells
	// viewer + " " + tmpfile through bash so the file is cleaned up
	// after the viewer exits. ext is the file extension without leading
	// dot (e.g. "md", "csv", "json"); empty falls back to "txt".
	Pager(content, viewer, ext string) error

	// SaveFile writes content to <home>/Downloads/<basename>-<ts>.<ext>
	// and returns the absolute path. The timestamp ensures no overwrite
	// of an earlier export. ext is without leading dot; empty falls back
	// to "txt". The Downloads directory is created if it doesn't exist.
	SaveFile(basename, ext string, content []byte) (path string, err error)
}
