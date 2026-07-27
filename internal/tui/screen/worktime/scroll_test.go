package worktime

import (
	"fmt"
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

func TestWindowRows_OverflowCountsAreConsistent(t *testing.T) {
	pal := theme.Load()
	mid := make([]string, 20)
	for i := range mid {
		mid[i] = fmt.Sprintf("r%d", i)
	}
	// focus mid-list so BOTH markers appear; budget 5, focus 15 -> start=13, end=18
	out := windowRows(mid, 15, 5, pal)
	if len(out) != 5 {
		t.Fatalf("want 5 rows, got %d", len(out))
	}
	top := plain(out[0])
	bot := plain(out[len(out)-1])
	// above the window: rows 0..12 fully hidden = 13
	if !strings.Contains(top, "13 darüber") {
		t.Fatalf("top marker = %q, want '13 darüber'", top)
	}
	// below the window: rows 18,19 fully hidden = 2 (consistent: excludes the displaced boundary row)
	if !strings.Contains(bot, "2 darunter") {
		t.Fatalf("bottom marker = %q, want '2 darunter'", bot)
	}
}
