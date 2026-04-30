package worktime

import (
	"strings"
	"testing"
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

// displayWidth approximates lipgloss.Width without depending on it directly
// (the test file is in package worktime so the import is fine; we keep a
// local helper to make the test independent of lipgloss style codes).
func displayWidth(s string) int {
	// Plain rune count is close enough for the ASCII-only test inputs above.
	n := 0
	for range s {
		n++
	}
	return n
}
