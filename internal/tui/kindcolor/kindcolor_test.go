package kindcolor_test

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestBadge_FixedWidthFiveCells(t *testing.T) {
	t.Parallel()
	for _, tp := range []domain.DocumentType{domain.DocDaily, domain.DocProject, domain.DocFree, domain.DocAgent} {
		if w := lipgloss.Width(kindcolor.Badge(tp)); w != 5 {
			t.Errorf("Badge(%q) width = %d, want 5", tp, w)
		}
	}
}

func TestColorAndGlyph_PerType(t *testing.T) {
	t.Parallel()
	p := theme.Default
	sem := p.Sem()
	cases := []struct {
		t     domain.DocumentType
		color theme.Color
		glyph string
	}{
		{domain.DocDaily, sem.Accent, "●"},
		{domain.DocProject, sem.Success, "◆"},
		{domain.DocFree, sem.Highlight, "○"},
		{domain.DocAgent, sem.Warning, "▪"},
	}
	for _, c := range cases {
		if got := kindcolor.Color(c.t, p); got != c.color {
			t.Errorf("Color(%q) = %q, want %q", c.t, got, c.color)
		}
		if got := kindcolor.Glyph(c.t); got != c.glyph {
			t.Errorf("Glyph(%q) = %q, want %q", c.t, got, c.glyph)
		}
	}
}
