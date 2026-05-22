package worktime

import (
	"image/color"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// TestTodayStatusBadge pins the Heute-headline colour contract:
//
//   - running && !achieved → Sem.Active (Cyan)  — cross-surface with
//     tmux pace dot + week pace strip
//   - running &&  achieved → Sem.Success         — running and Ziel done
//   - achieved             → Sem.Success         — idle and Ziel done
//   - else                 → FgMuted             — paused / empty
//
// A drift here means the Heute headline starts contradicting the
// other surfaces (review finding 2). Cross-surface alignment is the
// whole point, so it gets a dedicated test instead of relying on
// golden snapshots that would only flag the colour indirectly via
// ANSI escape diffs buried deep in the rendered string.
func TestTodayStatusBadge(t *testing.T) {
	p := theme.TokyonightNight
	sem := p.Sem()

	cases := []struct {
		name      string
		running   bool
		achieved  bool
		wantGlyph string
		wantLabel string
		wantColor color.Color
	}{
		{"running not achieved", true, false, glyphs.Active, "läuft", sem.Active},
		{"running achieved", true, true, glyphs.Active, "läuft " + glyphs.Done, sem.Success},
		{"idle achieved", false, true, glyphs.Done, "Ziel erreicht", sem.Success},
		{"idle not achieved", false, false, glyphs.Paused, "pausiert", p.FgMuted},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			glyph, label, color := todayStatusBadge(p, tt.running, tt.achieved)
			if glyph != tt.wantGlyph {
				t.Errorf("glyph: got %q, want %q", glyph, tt.wantGlyph)
			}
			if label != tt.wantLabel {
				t.Errorf("label: got %q, want %q", label, tt.wantLabel)
			}
			if color != tt.wantColor {
				t.Errorf("colour: got %v, want %v", color, tt.wantColor)
			}
		})
	}
}
