package worktime

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestFitHeight_WindowsAroundFocus(t *testing.T) {
	pal := theme.Load()
	header := []string{"H1", "H2"}
	mid := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		mid = append(mid, "row")
	}
	footer := []string{"F"}
	out := fitHeight(header, mid, footer, 15, 8, pal)
	lines := strings.Split(out, "\n")
	if len(lines) > 8 {
		t.Fatalf("over budget: %d lines", len(lines))
	}
	if lines[0] != "H1" || lines[len(lines)-1] != "F" {
		t.Fatalf("header/footer not pinned: %q…%q", lines[0], lines[len(lines)-1])
	}
}

func TestFitHeight_SmallBudgetNeverExceeds(t *testing.T) {
	pal := theme.Load()
	header := []string{"H1", "H2", "H3"}
	mid := []string{"m1", "m2", "m3"}
	footer := []string{"F"}
	for _, budget := range []int{0, 1, 2, 3} {
		out := fitHeight(header, mid, footer, 0, budget, pal)
		got := 0
		if out != "" {
			got = len(strings.Split(out, "\n"))
		}
		if got > budget {
			t.Fatalf("budget %d: got %d lines (exceeds budget)", budget, got)
		}
	}
}

func TestBodyBudget(t *testing.T) {
	if got := bodyBudget(20); got != 20 {
		t.Fatalf("bodyBudget = %d", got)
	}
}
