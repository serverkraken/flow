package statusbar_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

var testPalette = theme.Load()

func TestBar_ZeroPercent_AllEmpty(t *testing.T) {
	t.Parallel()
	b := statusbar.Bar(0, 10, testPalette)
	if strings.Contains(b, "▰") {
		t.Errorf("0%% bar should have no filled blocks: %q", b)
	}
	if lipgloss.Width(b) != 10 {
		t.Errorf("0%% bar width = %d, want 10", lipgloss.Width(b))
	}
}

func TestBar_FullPercent_AllFilled(t *testing.T) {
	t.Parallel()
	b := statusbar.Bar(100, 10, testPalette)
	if strings.Contains(b, "▱") {
		t.Errorf("100%% bar should have no empty blocks: %q", b)
	}
	if lipgloss.Width(b) != 10 {
		t.Errorf("100%% bar width = %d, want 10", lipgloss.Width(b))
	}
}

func TestBar_FiftyPercent_HalfFilled(t *testing.T) {
	t.Parallel()
	b := statusbar.Bar(50, 10, testPalette)
	got := lipgloss.Width(b)
	if got != 10 {
		t.Errorf("50%% bar width = %d, want 10", got)
	}
	// 5 filled + 5 empty
	filled := strings.Count(b, "▰")
	empty := strings.Count(b, "▱")
	if filled != 5 || empty != 5 {
		t.Errorf("50%% bar: got %d filled + %d empty, want 5+5", filled, empty)
	}
}

func TestBar_ClampsBelowZero(t *testing.T) {
	t.Parallel()
	b := statusbar.Bar(-10, 8, testPalette)
	if strings.Contains(b, "▰") {
		t.Errorf("negative pct bar should have no filled blocks: %q", b)
	}
}

func TestBar_ClampsAbove100(t *testing.T) {
	t.Parallel()
	b := statusbar.Bar(200, 8, testPalette)
	if strings.Contains(b, "▱") {
		t.Errorf("pct>100 bar should have no empty blocks: %q", b)
	}
}
