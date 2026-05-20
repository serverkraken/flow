package picker_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
)

// Edge-case branches in Row that the existing picker_test.go suite
// doesn't reach: very narrow widths drop the hint, and an even narrower
// width clamps the label budget to 1.

func TestRow_VeryNarrowWidth_DropsHint(t *testing.T) {
	t.Parallel()
	// width=5 reserves 3 cells for bar/space/gap; only 2 left for label
	// or hint. The label keeps priority, hint is dropped (maxLabel<1 branch).
	row := picker.Row(false, "longlabel", "h", 5, testPalette)
	if row == "" {
		t.Errorf("Row width=5 should still render, got empty")
	}
}

func TestRow_VerySmallWidth_StillRenders(t *testing.T) {
	t.Parallel()
	// width=3 → reserved=3, maxLabel = 0 → falls into the maxLabel<1
	// branch which drops hint and clamps maxLabel to 1.
	row := picker.Row(false, "x", "h", 3, testPalette)
	if row == "" {
		t.Errorf("Row width=3 should still render")
	}
}

func TestSectionHeader_TooNarrowGapZero(t *testing.T) {
	t.Parallel()
	// width less than the rendered "FOOBAR" length triggers the gap<0
	// clamp branch.
	h := picker.SectionHeader("foobar", 2, testPalette)
	if h == "" {
		t.Errorf("SectionHeader width=2 should still produce some output")
	}
}
