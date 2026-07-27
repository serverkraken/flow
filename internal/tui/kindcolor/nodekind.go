package kindcolor

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

// NodeKindColor maps a domain.NodeKind to its semantic hierarchy color. The
// kind is the organizing axis of the node tree, so the color is read from the
// palette's Semantic aliases (never a raw hue). Unknown kinds → neutral border.
func NodeKindColor(kind domain.NodeKind, p theme.Palette) theme.Color {
	sem := p.Sem()
	switch kind {
	case domain.KindEngagement:
		return sem.Accent
	case domain.KindVorhaben:
		return sem.Highlight
	case domain.KindRepo:
		return sem.Success
	case domain.KindBranch:
		return sem.Warning
	default:
		return sem.Border
	}
}

// NodeKindGlyph returns the single-cell identity glyph for a node kind, from the
// monospace whitelist. Distinct shape per kind so the tree is legible without
// color (A11y: shape + color, never color alone).
func NodeKindGlyph(kind domain.NodeKind) string {
	switch kind {
	case domain.KindEngagement:
		return glyphs.BarThick // ▌
	case domain.KindVorhaben:
		return glyphs.Bullet3 // ◆
	case domain.KindRepo:
		return glyphs.Filled // ●
	case domain.KindBranch:
		return glyphs.Bullet4 // ▪
	default:
		return glyphs.BulletDot // ·
	}
}

// NodeKindLabel returns the uppercase badge label for a node kind.
func NodeKindLabel(kind domain.NodeKind) string {
	switch kind {
	case domain.KindEngagement:
		return "ENGAGEMENT"
	case domain.KindVorhaben:
		return "VORHABEN"
	case domain.KindRepo:
		return "REPO"
	case domain.KindBranch:
		return "BRANCH"
	default:
		return "?"
	}
}
