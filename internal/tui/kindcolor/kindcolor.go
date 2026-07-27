// Package kindcolor maps a domain DocumentType to its visual identity (badge
// label, count glyph, semantic color). It is the single source of truth so a
// badge and its count glyph can never drift in color.
package kindcolor

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

// Badge returns a fixed-width (5-cell) badge label for the document type.
func Badge(t domain.DocumentType) string {
	switch t {
	case domain.DocDaily:
		return "TÄGL."
	case domain.DocProject:
		return "PROJ."
	case domain.DocFree:
		return "FREI "
	case domain.DocAgent:
		return "AGENT"
	default:
		return "  ?  "
	}
}

// Glyph returns the count/legend glyph for the document type.
func Glyph(t domain.DocumentType) string {
	switch t {
	case domain.DocDaily:
		return glyphs.CountDaily
	case domain.DocProject:
		return glyphs.CountProject
	case domain.DocFree:
		return glyphs.CountFree
	case domain.DocAgent:
		return glyphs.Bullet4
	default:
		return glyphs.BulletDot
	}
}

// Color returns the semantic color for the document type from the palette.
func Color(t domain.DocumentType, p theme.Palette) theme.Color {
	sem := p.Sem()
	switch t {
	case domain.DocDaily:
		return sem.Accent
	case domain.DocProject:
		return sem.Success
	case domain.DocFree:
		return sem.Highlight
	case domain.DocAgent:
		return sem.Warning
	default:
		return sem.Border
	}
}
