package usecase

import "github.com/serverkraken/flow/internal/ports"

// CheatsheetLoader reads the cheatsheet markdown source and runs it
// through the markdown renderer in one step. Width controls wrapping —
// the cheatsheet screen typically passes its viewport width.
type CheatsheetLoader struct {
	Reader   ports.CheatsheetReader
	Renderer ports.MarkdownRenderer
}

// LoadRaw returns the cheatsheet's untouched markdown source — useful
// when the caller wants to render or post-process it differently.
func (l *CheatsheetLoader) LoadRaw() (string, error) {
	return l.Reader.Load()
}

// Render returns the rendered (terminal-styled) cheatsheet at the given
// width. Returns an error if either the source load or the render fails.
func (l *CheatsheetLoader) Render(width int) (string, error) {
	raw, err := l.Reader.Load()
	if err != nil {
		return "", err
	}
	return l.Renderer.Render(raw, width)
}
