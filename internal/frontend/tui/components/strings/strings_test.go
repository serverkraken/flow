package strings_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	tuistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
)

func TestHintConfirm_UsesBracketedDefault(t *testing.T) {
	// A11y-6: default-Action bracketed `[y/Enter]` als non-color cue.
	// HintConfirm und confirm.View müssen denselben Stil sprechen.
	if !strings.Contains(tuistrings.HintConfirm, "[y/Enter]") {
		t.Errorf("HintConfirm: expected `[y/Enter]` brackets to match confirm.View, got %q", tuistrings.HintConfirm)
	}
}

func TestHintSearchInput_HasCanonicalShape(t *testing.T) {
	got := tuistrings.HintSearchInput
	for _, want := range []string{"tippen", "Enter", "anwenden", "Esc", "abbrechen", "→"} {
		if !strings.Contains(got, want) {
			t.Errorf("HintSearchInput %q missing %q", got, want)
		}
	}
	if !strings.Contains(got, "  ·  ") {
		t.Errorf("HintSearchInput missing canonical `  ·  ` separator")
	}
}

func TestTruncate_NoOpWhenWithinWidth(t *testing.T) {
	t.Parallel()
	got := tuistrings.Truncate("hello", 10)
	if got != "hello" {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestTruncate_ClipsAndAppendsEllipsis(t *testing.T) {
	t.Parallel()
	got := tuistrings.Truncate("abcdefghij", 5)
	if got != "abcd…" {
		t.Errorf("expected abcd…, got %q", got)
	}
	if w := lipgloss.Width(got); w != 5 {
		t.Errorf("width should be exactly 5, got %d", w)
	}
}

func TestTruncate_ZeroAndNegativeReturnEmpty(t *testing.T) {
	t.Parallel()
	if got := tuistrings.Truncate("anything", 0); got != "" {
		t.Errorf("0 → empty, got %q", got)
	}
	if got := tuistrings.Truncate("anything", -3); got != "" {
		t.Errorf("-3 → empty, got %q", got)
	}
}

func TestTruncate_WidthOneOnlyEllipsis(t *testing.T) {
	t.Parallel()
	if got := tuistrings.Truncate("abc", 1); got != "…" {
		t.Errorf("width 1 → '…', got %q", got)
	}
}

func TestTruncate_RuneAware(t *testing.T) {
	t.Parallel()
	// Multi-byte runes must count as cells, not bytes. The umlauts are
	// 1 cell each; the truncate must include them and stop at the cell
	// boundary — never cut mid-byte.
	got := tuistrings.Truncate("Heütefäng", 5)
	if w := lipgloss.Width(got); w != 5 {
		t.Errorf("rune-aware width should equal 5, got %d (%q)", w, got)
	}
}
