package ports

// MarkdownRenderer turns a Markdown source string into a styled,
// terminal-ready string. The default implementation lives in
// internal/frontend/tui/markdown (a goldmark + chroma + lipgloss
// pipeline); width is the target column count for wrapping.
type MarkdownRenderer interface {
	Render(content string, width int) (string, error)
}
