package kindcolor_test

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestNodeKind_ColorGlyphLabel(t *testing.T) {
	t.Parallel()
	p := theme.Default
	sem := p.Sem()
	cases := []struct {
		kind  domain.NodeKind
		color theme.Color
		glyph string
		label string
	}{
		{domain.KindEngagement, sem.Accent, "▌", "ENGAGEMENT"},
		{domain.KindVorhaben, sem.Highlight, "◆", "VORHABEN"},
		{domain.KindRepo, sem.Success, "●", "REPO"},
		{domain.KindBranch, sem.Warning, "▪", "BRANCH"},
	}
	for _, c := range cases {
		if got := kindcolor.NodeKindColor(c.kind, p); got != c.color {
			t.Errorf("NodeKindColor(%q) = %q, want %q", c.kind, got, c.color)
		}
		if got := kindcolor.NodeKindGlyph(c.kind); got != c.glyph {
			t.Errorf("NodeKindGlyph(%q) = %q, want %q", c.kind, got, c.glyph)
		}
		if got := kindcolor.NodeKindLabel(c.kind); got != c.label {
			t.Errorf("NodeKindLabel(%q) = %q, want %q", c.kind, got, c.label)
		}
	}
}

func TestNodeKindGlyph_SingleCell(t *testing.T) {
	t.Parallel()
	for _, k := range []domain.NodeKind{
		domain.KindEngagement, domain.KindVorhaben, domain.KindRepo, domain.KindBranch,
	} {
		if w := lipgloss.Width(kindcolor.NodeKindGlyph(k)); w != 1 {
			t.Errorf("NodeKindGlyph(%q) width = %d, want 1 (monospace whitelist)", k, w)
		}
	}
}

func TestNodeKind_UnknownFallback(t *testing.T) {
	t.Parallel()
	p := theme.Default
	if got := kindcolor.NodeKindColor(domain.NodeKind("bogus"), p); got != p.Sem().Border {
		t.Errorf("unknown kind color = %q, want Border", got)
	}
	if got := kindcolor.NodeKindGlyph(domain.NodeKind("bogus")); got != "·" {
		t.Errorf("unknown kind glyph = %q, want ·", got)
	}
}
