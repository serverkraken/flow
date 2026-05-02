package ports

// CheatsheetReader returns the raw Markdown source of the cheatsheet
// (~/.tmux/cheatsheet.md or wherever the adapter resolves it from).
// Rendering is the MarkdownRenderer's job, kept separate so the same
// content could be piped through different renderers.
type CheatsheetReader interface {
	Load() (string, error)
}
