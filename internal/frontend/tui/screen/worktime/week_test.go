package worktime

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
)

// Under lipgloss v2 Style.Render always emits TrueColor SGR
// sequences regardless of TTY detection — no TestMain profile
// override needed.

// TestRenderPace_FreeDayUsesFilledGlyphPerKindColor pinnt fest, dass die
// Pace-Strip für jeden Free-Day-Kind den ●-Glyph (glyphs.Filled) emittiert
// und die Foreground-Farbe per Kind via Sem.Schedule/Highlight/Notice
// trägt. Cross-surface identisch mit dem tmux-Bar.
// Spec: docs/superpowers/specs/2026-05-13-filled-dayoff-dots-supersede.md
func TestRenderPace_FreeDayUsesFilledGlyphPerKindColor(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local) // Fri
	fri := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	pal := theme.TokyonightNight

	// colorSeq converts a lipgloss.Color to its ANSI foreground sequence
	// as emitted by lipgloss in TrueColor mode.
	colorSeq := func(c theme.Color) string {
		return ansiFG(c)
	}

	// Spec 2026-05-13-filled-dayoff-dots-supersede: direct hue mapping.
	tests := []struct {
		kind domain.Kind
		seq  string
	}{
		{domain.KindHoliday, colorSeq(pal.Blue)},
		{domain.KindVacation, colorSeq(pal.Purple)},
		{domain.KindSick, colorSeq(pal.Orange)},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			dayoffs := testutil.NewFakeDayOffStore()
			if err := dayoffs.Add(domain.DayOff{Date: fri, Kind: tc.kind, Label: "T"}); err != nil {
				t.Fatalf("seed: %v", err)
			}
			deps := Deps{
				DayOffStore: dayoffs,
				Clock:       &testutil.FixedClock{T: now},
			}
			w := newWoche(pal, deps)
			w.week = []domain.WeekDay{{Date: fri, Target: 8 * time.Hour}}
			w.loaded = true
			w.width = 80
			out := w.renderPace(now)
			// Spec 2026-05-13-filled-dayoff-dots-supersede: ● for day-offs
			// (cross-surface with tmux); kind colour distinguishes which.
			if !strings.Contains(out, glyphs.Filled) {
				t.Errorf("renderPace should contain %q for free day, got: %q", glyphs.Filled, out)
			}
			if !strings.Contains(out, tc.seq) {
				t.Errorf("renderPace should contain ANSI seq %q for kind %q, got: %q", tc.seq, tc.kind, out)
			}
		})
	}
}
