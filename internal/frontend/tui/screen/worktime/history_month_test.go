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

// TestRenderMonthCell_FreeDayColoredPerKind pinnt fest, dass die Monats-
// Grid-Zelle für freie Tage den ○-Glyph trägt und die Farbe per Kind
// (Info/Success/Warning) statt pauschal Info benutzt.
// Spec: docs/superpowers/specs/2026-05-12-unified-dayoff-glyphs-design.md
func TestRenderMonthCell_FreeDayColoredPerKind(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	day := time.Date(2026, 5, 4, 0, 0, 0, 0, time.Local) // Mon, in May
	pal := theme.TokyonightNight

	colorSeq := func(c theme.Color) string {
		return ansiFG(c)
	}

	// Spec 2026-05-13-filled-dayoff-dots-supersede: direct hue mapping.
	tests := []struct {
		kind  domain.Kind
		color string
	}{
		{domain.KindHoliday, colorSeq(pal.Blue)},
		{domain.KindVacation, colorSeq(pal.Purple)},
		{domain.KindSick, colorSeq(pal.Orange)},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			dayoffs := testutil.NewFakeDayOffStore()
			if err := dayoffs.Add(domain.DayOff{Date: day, Kind: tc.kind, Label: "T"}); err != nil {
				t.Fatalf("seed: %v", err)
			}
			deps := Deps{
				DayOffStore: dayoffs,
				Clock:       &testutil.FixedClock{T: now},
			}
			// Cursor weg vom Test-Day (monthCur=99) damit der Cursor-Style
			// die Per-Kind-Farb-Erwartung nicht überschreibt.
			h := history{pal: pal, deps: deps, monthCur: 99}
			byKey := map[string]domain.DayRecord{}
			out := h.renderMonthCell(day, true, byKey, day)
			if !strings.Contains(out, glyphs.Filled) {
				t.Errorf("cell missing %q for kind %q: %q", glyphs.Filled, tc.kind, out)
			}
			if !strings.Contains(out, tc.color) {
				t.Errorf("cell should colour kind %q with %q, got: %q", tc.kind, tc.color, out)
			}
		})
	}
}
