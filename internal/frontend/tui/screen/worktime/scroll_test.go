package worktime

// White-box tests for the height-windowing helper. Black-box can't reach
// the unexported fitHeight / windowRows, and a full View() test (see
// render_repro_test.go's height variant) is too indirect to pin the
// marker / focus-visibility edge cases.

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func TestFitHeight_FitsUnchanged(t *testing.T) {
	t.Parallel()
	pal := theme.Load()
	header := []string{"H1", "H2"}
	mid := []string{"M1", "M2", "M3"}
	footer := []string{"F1"}
	out := fitHeight(header, mid, footer, 0, 30, pal)
	got := lines(out)
	want := []string{"H1", "H2", "M1", "M2", "M3", "F1"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
	if strings.Contains(out, glyphs.Up) || strings.Contains(out, glyphs.Down) {
		t.Errorf("no overflow markers expected when content fits:\n%s", out)
	}
}

func TestFitHeight_NeverExceedsBudget(t *testing.T) {
	t.Parallel()
	pal := theme.Load()
	header := []string{"H1", "H2"}
	footer := []string{"F1", "F2"}
	mid := make([]string, 40)
	for i := range mid {
		mid[i] = "row"
	}
	for budget := 1; budget <= 50; budget++ {
		out := fitHeight(header, mid, footer, 20, budget, pal)
		if n := len(lines(out)); n > budget {
			t.Errorf("budget %d: produced %d lines", budget, n)
		}
	}
}

func TestFitHeight_OverflowShowsMarkersAndKeepsFocus(t *testing.T) {
	t.Parallel()
	pal := theme.Load()
	header := []string{"HEAD"}
	footer := []string{"FOOT"}
	mid := make([]string, 30)
	for i := range mid {
		mid[i] = "row"
	}
	const focus = 15
	mid[focus] = "FOCUS-ROW"

	out := fitHeight(header, mid, footer, focus, 12, pal)
	got := lines(out)
	if len(got) != 12 {
		t.Fatalf("expected exactly 12 lines, got %d:\n%s", len(got), out)
	}
	if got[0] != "HEAD" {
		t.Errorf("header not pinned at top: %q", got[0])
	}
	if got[len(got)-1] != "FOOT" {
		t.Errorf("footer not pinned at bottom: %q", got[len(got)-1])
	}
	if !strings.Contains(out, "FOCUS-ROW") {
		t.Errorf("focus row scrolled out of view:\n%s", out)
	}
	if !strings.Contains(out, glyphs.Up) {
		t.Errorf("expected ▲ overflow marker (rows hidden above):\n%s", out)
	}
	if !strings.Contains(out, glyphs.Down) {
		t.Errorf("expected ▼ overflow marker (rows hidden below):\n%s", out)
	}
}

func TestFitHeight_FocusVisibleAcrossWholeRange(t *testing.T) {
	t.Parallel()
	pal := theme.Load()
	header := []string{"HEAD"}
	footer := []string{"FOOT"}
	n := 40
	for focus := 0; focus < n; focus++ {
		mid := make([]string, n)
		for i := range mid {
			mid[i] = "row"
		}
		mid[focus] = "FOCUS"
		out := fitHeight(header, mid, footer, focus, 10, pal)
		if !strings.Contains(out, "FOCUS") {
			t.Errorf("focus %d scrolled out of view:\n%s", focus, out)
		}
	}
}

func TestFitHeight_TopAndBottomNoFalseMarkers(t *testing.T) {
	t.Parallel()
	pal := theme.Load()
	header := []string{"HEAD"}
	footer := []string{"FOOT"}
	n := 30
	mid := make([]string, n)
	for i := range mid {
		mid[i] = "row"
	}
	// Focus at very top: nothing hidden above → no ▲ marker.
	top := fitHeight(header, mid, footer, 0, 12, pal)
	if strings.Contains(top, glyphs.Up) {
		t.Errorf("focus at top should not show a ▲ (hidden-above) marker:\n%s", top)
	}
	if !strings.Contains(top, glyphs.Down) {
		t.Errorf("focus at top should still show a ▼ (hidden-below) marker:\n%s", top)
	}
	// Focus at very bottom: nothing hidden below → no ▼ marker.
	bot := fitHeight(header, mid, footer, n-1, 12, pal)
	if strings.Contains(bot, glyphs.Down) {
		t.Errorf("focus at bottom should not show a ▼ (hidden-below) marker:\n%s", bot)
	}
	if !strings.Contains(bot, glyphs.Up) {
		t.Errorf("focus at bottom should still show a ▲ (hidden-above) marker:\n%s", bot)
	}
}

func TestFitHeight_TinyBudgetKeepsHeader(t *testing.T) {
	t.Parallel()
	pal := theme.Load()
	header := []string{"HEAD-A", "HEAD-B", "HEAD-C"}
	mid := []string{"M1", "M2"}
	footer := []string{"F1"}
	out := fitHeight(header, mid, footer, 0, 1, pal)
	got := lines(out)
	if len(got) != 1 {
		t.Fatalf("budget 1 should yield exactly 1 line, got %d: %#v", len(got), got)
	}
	if got[0] != "HEAD-A" {
		t.Errorf("budget 1 should keep the first header row, got %q", got[0])
	}
	if fitHeight(header, mid, footer, 0, 0, pal) != "" {
		t.Errorf("budget 0 should render nothing")
	}
}
