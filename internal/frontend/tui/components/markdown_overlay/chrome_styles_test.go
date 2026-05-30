package markdown_overlay

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// TestBuildStyles_FrameUsesBorderStrong pins the overlay's load-bearing
// frame to Sem.BorderStrong (≥3:1 WCAG-non-text). Skill §Component
// vocabulary: load-bearing frames carry BorderStrong, not Accent.
// Previously Frame and Title both took Sem.Accent, colliding with the
// cursor style (also Accent) and breaking single-accent-per-row.
func TestBuildStyles_FrameUsesBorderStrong(t *testing.T) {
	p := theme.TokyonightNight
	cs := buildStyles(p)
	got := cs.frame.GetBorderTopForeground()
	want := p.Sem().BorderStrong
	if got != want {
		t.Errorf("frame BorderForeground = %v, want %v (BorderStrong, Skill load-bearing)", got, want)
	}
}

// TestBuildStyles_TitleUsesHighlight pins the overlay's title to
// Sem.Highlight (Purple + Bold) per the Skill titlebox convention.
// Highlight is the canonical title color across the app; using Accent
// here duplicated the cursor's accent and broke the single-accent-per-row rule.
func TestBuildStyles_TitleUsesHighlight(t *testing.T) {
	p := theme.TokyonightNight
	cs := buildStyles(p)
	got := cs.title.GetForeground()
	want := p.Sem().Highlight
	if got != want {
		t.Errorf("title fg = %v, want %v (Highlight per titlebox convention)", got, want)
	}
}
