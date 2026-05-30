package picker_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// TestRowWithMatch_NoMatch_EqualToPickerRow proves the contract: an
// empty Match slice produces byte-identical output to plain Row. This
// is the migration safety net — palette/projects callers can switch in
// without visual drift for entries the fuzzy filter didn't touch.
func TestRowWithMatch_NoMatch_EqualToPickerRow(t *testing.T) {
	t.Parallel()
	p := theme.TokyonightNight
	wm := picker.RowWithMatch(picker.RowWithMatchOpts{
		Selected: true, Label: "Heute", Hint: "▶", Width: 20, Match: nil,
	}, p)
	plain := picker.Row(true, "Heute", "▶", 20, p)
	if wm != plain {
		t.Errorf("RowWithMatch(no match): expected equal to Row(...).\n  wm:    %q\n  plain: %q", wm, plain)
	}
}

// TestRowWithMatch_HighlightsAtIndices proves match-runes get the
// Sem.Accent SGR sequence (Bold is asserted only structurally — by
// presence of the accent color in the output, which palette/projects
// also rely on).
func TestRowWithMatch_HighlightsAtIndices(t *testing.T) {
	t.Parallel()
	p := theme.TokyonightNight
	opts := picker.RowWithMatchOpts{
		Selected: true,
		Label:    "Heute",
		Hint:     "▶",
		Width:    20,
		Match:    []int{0, 2},
	}
	out := picker.RowWithMatch(opts, p)
	rPalette := p.Sem().Accent
	if !containsSemSGR(out, rPalette) {
		t.Errorf("RowWithMatch: expected Sem.Accent SGR sequence in output, got %q", out)
	}
}

// containsSemSGR checks the truecolor SGR triplet for c appears
// somewhere in out — matches lipgloss v2 ANSI emission.
func containsSemSGR(out string, c theme.Color) bool {
	hex := string(c)
	if len(hex) != 7 || hex[0] != '#' {
		return false
	}
	r, _ := strconv.ParseInt(hex[1:3], 16, 0)
	g, _ := strconv.ParseInt(hex[3:5], 16, 0)
	b, _ := strconv.ParseInt(hex[5:7], 16, 0)
	rgb := fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
	return strings.Contains(out, rgb)
}
