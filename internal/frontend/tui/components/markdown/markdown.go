// Package markdown provides a glamour-based Markdown renderer with
// Tokyonight-appropriate defaults.
package markdown

import (
	"github.com/charmbracelet/glamour"
)

// Render converts Markdown source to styled terminal output.
// width controls word-wrap. On any glamour error the raw source is returned.
func Render(source string, width int) (string, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return source, err
	}
	out, err := r.Render(source)
	if err != nil {
		return source, err
	}
	return out, nil
}
