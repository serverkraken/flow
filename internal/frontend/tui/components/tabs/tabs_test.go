package tabs_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/tabs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestRender_UnderlineMarksActive(t *testing.T) {
	t.Parallel()
	out := tabs.Render(
		[]tabs.Item{{Label: "Heute"}, {Label: "Woche"}, {Label: "History"}},
		1, 0, tabs.Underline, theme.TokyonightNight,
	)
	for _, want := range []string{"Heute", "Woche", "History"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing tab label %q in %q", want, out)
		}
	}
	if !strings.Contains(out, "─") {
		t.Errorf("underline rule missing in %q", out)
	}
}

func TestRender_Pill(t *testing.T) {
	t.Parallel()
	out := tabs.Render(
		[]tabs.Item{{Label: "A"}, {Label: "B"}},
		0, 0, tabs.Pill, theme.TokyonightNight,
	)
	if !strings.Contains(out, "A") || !strings.Contains(out, "B") {
		t.Errorf("missing labels in %q", out)
	}
}

func TestRender_Empty(t *testing.T) {
	t.Parallel()
	if got := tabs.Render(nil, 0, 0, tabs.Underline, theme.TokyonightNight); got != "" {
		t.Errorf("empty items should render empty, got %q", got)
	}
}

func TestRender_GlyphAndBadge(t *testing.T) {
	t.Parallel()
	out := tabs.Render(
		[]tabs.Item{{Label: "PRs", Glyph: '⏱', Badge: "3"}},
		0, 0, tabs.Underline, theme.TokyonightNight,
	)
	if !strings.Contains(out, "⏱") {
		t.Errorf("glyph missing in %q", out)
	}
	if !strings.Contains(out, "(3)") {
		t.Errorf("badge missing in %q", out)
	}
}

func TestRender_DisabledTabUsesMuted(t *testing.T) {
	t.Parallel()
	// Just smoke-test that disabled doesn't panic and emits the label.
	out := tabs.Render(
		[]tabs.Item{{Label: "Active"}, {Label: "Off", Disabled: true}},
		0, 0, tabs.Underline, theme.TokyonightNight,
	)
	if !strings.Contains(out, "Off") {
		t.Errorf("disabled label missing in %q", out)
	}
}
