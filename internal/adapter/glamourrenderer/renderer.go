package glamourrenderer

import (
	"github.com/charmbracelet/glamour"
)

// Renderer wraps glamour as a MarkdownRenderer.
type Renderer struct{}

// New constructs a Renderer. The zero value works equally well.
func New() Renderer { return Renderer{} }

// Render returns the styled output. width controls glamour's word wrap;
// pass <= 2 to disable wrapping (glamour requires width >= 3, so
// anything smaller falls back to no wrap).
func (Renderer) Render(content string, width int) (string, error) {
	opts := []glamour.TermRendererOption{
		glamour.WithStylePath("dark"),
	}
	if width > 2 {
		opts = append(opts, glamour.WithWordWrap(width-2))
	}
	r, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return content, err
	}
	out, err := r.Render(content)
	if err != nil {
		return content, err
	}
	return out, nil
}
