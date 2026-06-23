package domain_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestProjectColorGlyphWhitelist(t *testing.T) {
	if len(domain.ProjectColors) == 0 || len(domain.ProjectGlyphs) == 0 {
		t.Fatal("whitelists must be non-empty")
	}
	// "" (unset) is always valid; a whitelisted value is valid; junk is not.
	if !domain.ValidProjectColor("") || !domain.ValidProjectGlyph("") {
		t.Error("empty must be valid (optional fields)")
	}
	if !domain.ValidProjectColor(domain.ProjectColors[0]) {
		t.Errorf("first palette color %q must validate", domain.ProjectColors[0])
	}
	if !domain.ValidProjectGlyph(domain.ProjectGlyphs[0]) {
		t.Errorf("first glyph %q must validate", domain.ProjectGlyphs[0])
	}
	if domain.ValidProjectColor("chartreuse") || domain.ValidProjectGlyph("Z") {
		t.Error("junk color/glyph must not validate")
	}
}

func TestProjectValidateColorGlyph(t *testing.T) {
	base := domain.Project{Name: "Flow", Slug: "flow", Status: domain.ProjectActive}
	t.Run("empty color/glyph ok", func(t *testing.T) {
		if err := base.Validate(); err != nil {
			t.Errorf("unset color/glyph must be valid: %v", err)
		}
	})
	t.Run("whitelisted ok", func(t *testing.T) {
		p := base
		p.Color = domain.ProjectColors[0]
		p.Glyph = domain.ProjectGlyphs[0]
		if err := p.Validate(); err != nil {
			t.Errorf("whitelisted color/glyph must be valid: %v", err)
		}
	})
	t.Run("bad color rejected", func(t *testing.T) {
		p := base
		p.Color = "chartreuse"
		if !errors.Is(p.Validate(), domain.ErrInvalidProject) {
			t.Error("want ErrInvalidProject for non-whitelist color")
		}
	})
	t.Run("bad glyph rejected", func(t *testing.T) {
		p := base
		p.Glyph = "Z"
		if !errors.Is(p.Validate(), domain.ErrInvalidProject) {
			t.Error("want ErrInvalidProject for non-whitelist glyph")
		}
	})
}
