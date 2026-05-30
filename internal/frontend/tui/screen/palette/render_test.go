package palette

// Internal (white-box) tests for the render-level glyph semantics.
// Skill §Glyph whitelist: glyphs.Active (▶) bedeutet "running / live".
// Die Palette darf es weder für Filter-Focus (focus ≠ live) noch für
// die Preview-Zeile (about-to-run ≠ currently-running) verwenden.

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestRenderPrompt_FocusedDoesNotUseActiveGlyph(t *testing.T) {
	p := theme.TokyonightNight
	m := Model{pal: p, width: 80, styles: newPaletteStyles(p)}
	m.filter = form.NewTextInput("…", p)
	m.filter.Focus()
	out := m.viewContent()
	// glyphs.Active (▶) ist Skill §Glyph whitelist "running/live",
	// nicht "Focus". Der focused-Prompt darf ihn nicht tragen.
	if strings.Contains(out, glyphs.Active) {
		t.Errorf("focused prompt: must not use glyphs.Active (▶ = running, not focus); got %q", out)
	}
	// glyphs.Info (›) als Fokus-Marker, Accent+Bold via Heading.
	if !strings.Contains(out, glyphs.Info) {
		t.Errorf("focused prompt: expected glyphs.Info (›) marker")
	}
}

func TestRenderPreview_PrefixUsesAccentBarNotActive(t *testing.T) {
	p := theme.TokyonightNight
	m := Model{
		pal: p, width: 80, styles: newPaletteStyles(p),
		visible: []domain.PaletteEntry{{Label: "Test", Action: "echo hi"}},
	}
	out := m.renderPreview(76)
	if strings.Contains(out, glyphs.Active) {
		t.Errorf("preview: must not use glyphs.Active (▶ = running, preview is future-action)")
	}
	if !strings.Contains(out, glyphs.AccentBar) {
		t.Errorf("preview: expected glyphs.AccentBar (▎) as preview marker")
	}
}
