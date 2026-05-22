package worktime

// White-box tests for package-private formatting helpers (joinWrapped,
// stDim). Black-box (worktime_test) can't reach unexported funcs and
// View()-driven coverage would be too indirect for these pure helpers.

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestJoinWrapped_FitsOnOneLine(t *testing.T) {
	got := joinWrapped([]string{"a", "b", "c"}, "  ·  ", "  ", "  ", 80)
	want := "  a  ·  b  ·  c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestJoinWrapped_BreaksWhenTooLong(t *testing.T) {
	parts := []string{
		"Streak 12",
		"+1h 24m vs Schnitt",
		"Monat 35h 00m / 40h 00m -5h 00m",
		"Pause 1h 30m  (max 45m)",
		"fertig typisch 17:38",
	}
	out := joinWrapped(parts, "  ·  ", "  ", "  ", 60)
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d:\n%s", len(lines), out)
	}
	for _, l := range lines {
		if displayWidth(l) > 60 {
			t.Errorf("line exceeds 60 columns: %q (width %d)", l, displayWidth(l))
		}
	}
}

func TestJoinWrapped_DisabledWhenNoWidth(t *testing.T) {
	got := joinWrapped([]string{"a", "b", "c"}, " · ", "", "", 0)
	want := "a · b · c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestJoinWrapped_SinglePartLongerThanWidth(t *testing.T) {
	// One part that's already longer than maxWidth should still be emitted —
	// the helper can't split a single chip into two lines, only wrap between
	// chips. Otherwise we'd silently lose data.
	got := joinWrapped([]string{"this-chip-is-way-too-long-for-the-budget"}, " · ", "", "", 10)
	if !strings.Contains(got, "this-chip-is-way-too-long-for-the-budget") {
		t.Errorf("oversize single part dropped: %q", got)
	}
}

// displayWidth counts runes — close enough for the ASCII-only test inputs
// above. Keeps the helper independent of lipgloss style codes.
func displayWidth(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// TestStDimMultilinePadsShorterLines pins the lipgloss behaviour we rely
// on: when a string passed through lipgloss.Render contains a newline,
// the shorter line gets padded with spaces to match the longer one. The
// padding leaks into preceding output via plain string concatenation, so
// production callers must keep "\n" *outside* stDim. If lipgloss ever
// changes this, the test fails and the workaround can be simplified.
func TestStDimMultilinePadsShorterLines(t *testing.T) {
	out := stDim(theme.Palette{}, "\n  short.")
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), out)
	}
	if lipgloss.Width(lines[0]) == 0 {
		t.Skip("lipgloss no longer pads multi-line styled strings — the workaround can be simplified.")
	}
}
