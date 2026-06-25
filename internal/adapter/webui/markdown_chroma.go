package webui

import (
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
)

// highlightingExtension renders fenced code blocks with Chroma using CSS
// classes. The visual theme is supplied by static/chroma.css.
func highlightingExtension() goldmark.Extender {
	return highlighting.NewHighlighting(
		highlighting.WithFormatOptions(
			chromahtml.WithClasses(true),
			chromahtml.ClassPrefix(""),
		),
	)
}
