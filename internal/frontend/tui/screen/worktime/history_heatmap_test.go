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

// TestRenderHeatmapCell_FreeDayColoredPerKind pinnt fest, dass die
// Heatmap-Zelle für freie Tage jetzt ○ und die Farbe per Kind trägt
// (Info/Success/Warning) statt pauschal Sem.Info wie zuvor.
// Spec: docs/superpowers/specs/2026-05-12-unified-dayoff-glyphs-design.md
func TestRenderHeatmapCell_FreeDayColoredPerKind(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	day := time.Date(2026, 5, 4, 0, 0, 0, 0, time.Local) // Mon
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
			// Cursor weg vom Test-Day (w=99, d=99), damit Cursor-Style
			// nicht die Per-Kind-Farb-Erwartung überschreibt.
			h := history{pal: pal, deps: deps, styles: newHistoryStyles(pal)}
			byKey := map[string]domain.DayRecord{}
			out := h.renderHeatmapCell(day, byKey, 99, 99, now)
			if !strings.Contains(out, " "+glyphs.Filled+" ") {
				t.Errorf("cell missing ' %s ' for kind %q: %q", glyphs.Filled, tc.kind, out)
			}
			if !strings.Contains(out, tc.color) {
				t.Errorf("cell should colour kind %q with %q, got: %q", tc.kind, tc.color, out)
			}
		})
	}
}

// TestHeatmapCell_NoPerCellStyleAllocation pins the P5.1 perf refactor:
// the per-cell `lipgloss.NewStyle()` in renderHeatmapCell (was 26×7 = 182
// allocs per heatmap frame) is replaced by a pre-built style cache on
// historyStyles. The test asserts the cache fields exist and carry the
// correct Sem-token foregrounds so any future palette swap propagates.
func TestHeatmapCell_NoPerCellStyleAllocation(t *testing.T) {
	pal := theme.TokyonightNight
	h := history{
		pal:    pal,
		styles: newHistoryStyles(pal),
	}
	if h.styles.heatStepStyle[1.0].GetForeground() != pal.Sem().Success {
		t.Errorf("heatStepStyle[1.0]: expected Sem.Success preloaded, got %v",
			h.styles.heatStepStyle[1.0].GetForeground())
	}
	if h.styles.heatEmptyStyle.GetForeground() != pal.Sem().Border {
		t.Errorf("heatEmptyStyle: expected Sem.Border preloaded, got %v",
			h.styles.heatEmptyStyle.GetForeground())
	}
}

// TestRenderHeatmapLegend_ThreeColoredKindChips: Legende zeigt drei
// separate ○-Chips für die drei Kinds in den jeweiligen Sem-Farben.
func TestRenderHeatmapLegend_ThreeColoredKindChips(t *testing.T) {
	pal := theme.TokyonightNight

	colorSeq := func(c theme.Color) string {
		return ansiFG(c)
	}

	h := history{pal: pal, styles: newHistoryStyles(pal)}
	out := h.renderHeatmapLegend(120)
	wants := []struct {
		label string
		color string
	}{
		{"● Feiertag", colorSeq(pal.Blue)},
		{"● Urlaub", colorSeq(pal.Purple)},
		{"● Krank", colorSeq(pal.Orange)},
	}
	for _, w := range wants {
		if !strings.Contains(out, w.label) {
			t.Errorf("legend missing %q: %q", w.label, out)
		}
		if !strings.Contains(out, w.color) {
			t.Errorf("legend missing colour %q for %q: %q", w.color, w.label, out)
		}
	}
}
