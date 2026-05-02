package ports

// MarkdownRenderer turns a Markdown source string into a styled,
// terminal-ready string. Adapters typically use charmbracelet/glamour;
// width is the target column count for wrapping.
type MarkdownRenderer interface {
	Render(content string, width int) (string, error)
}
