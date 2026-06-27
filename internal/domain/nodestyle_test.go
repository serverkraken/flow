package domain_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestProjectColorGlyphWhitelist(t *testing.T) {
	if len(domain.NodeColors) == 0 || len(domain.NodeGlyphs) == 0 {
		t.Fatal("whitelists must be non-empty")
	}
	// "" (unset) is always valid; a whitelisted value is valid; junk is not.
	if !domain.ValidNodeColor("") || !domain.ValidNodeGlyph("") {
		t.Error("empty must be valid (optional fields)")
	}
	if !domain.ValidNodeColor(domain.NodeColors[0]) {
		t.Errorf("first palette color %q must validate", domain.NodeColors[0])
	}
	if !domain.ValidNodeGlyph(domain.NodeGlyphs[0]) {
		t.Errorf("first glyph %q must validate", domain.NodeGlyphs[0])
	}
	if domain.ValidNodeColor("chartreuse") || domain.ValidNodeGlyph("Z") {
		t.Error("junk color/glyph must not validate")
	}
}

func TestProjectValidateColorGlyph(t *testing.T) {
	base := domain.Node{Name: "Flow", Slug: "flow", Status: domain.NodeActive}
	t.Run("empty color/glyph ok", func(t *testing.T) {
		if err := base.Validate(); err != nil {
			t.Errorf("unset color/glyph must be valid: %v", err)
		}
	})
	t.Run("whitelisted ok", func(t *testing.T) {
		p := base
		p.Color = domain.NodeColors[0]
		p.Glyph = domain.NodeGlyphs[0]
		if err := p.Validate(); err != nil {
			t.Errorf("whitelisted color/glyph must be valid: %v", err)
		}
	})
	t.Run("bad color rejected", func(t *testing.T) {
		p := base
		p.Color = "chartreuse"
		if !errors.Is(p.Validate(), domain.ErrInvalidNode) {
			t.Error("want ErrInvalidNode for non-whitelist color")
		}
	})
	t.Run("bad glyph rejected", func(t *testing.T) {
		p := base
		p.Glyph = "Z"
		if !errors.Is(p.Validate(), domain.ErrInvalidNode) {
			t.Error("want ErrInvalidNode for non-whitelist glyph")
		}
	})
}
