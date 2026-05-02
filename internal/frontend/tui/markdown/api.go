package markdown

import "github.com/serverkraken/flow/internal/ports"

// Renderer is the option-less markdown renderer typed for
// ports.MarkdownRenderer. Surfaces that don't need a wikilink resolver
// (cheatsheet today; any future plain-Markdown viewer) can wire one of
// these via composition root and stay decoupled from the markdown
// package's full Render signature.
//
// Wikilinks render as broken in this mode (no resolver wired); pass
// the kompendium adapter at internal/kompendium/adapter/wikilinkresolver
// to the markdown.Render function directly when you need them resolved.
type Renderer struct{}

// NewRenderer constructs a Renderer. The zero value is equivalent.
func NewRenderer() Renderer { return Renderer{} }

// Render implements ports.MarkdownRenderer by delegating to the
// package-level Render with no options.
func (Renderer) Render(content string, width int) (string, error) {
	return Render(content, width)
}

var _ ports.MarkdownRenderer = Renderer{}
